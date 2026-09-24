package application

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"

	"go-proxy/internal/config"
	"go-proxy/internal/store"
)

// maxCacheFile bounds the copy of sing-box's cache. The file holds rule sets
// and a DNS reject cache; tens of megabytes would already be far outside what
// the rule-set catalogue produces.
const maxCacheFile = 64 << 20

// ruleSetFiles are the rule sets route test can look into, each as a file
// `sing-box rule-set match` reads. Remote sets come out of sing-box's own
// cache, which is where sing-box keeps what it downloaded, so the test needs no
// network and matches against exactly the data the running server uses.
type ruleSetFiles struct {
	dir    string
	path   map[string]string
	format map[string]string
}

func (f *ruleSetFiles) Close() {
	if f.dir != "" {
		_ = os.RemoveAll(f.dir)
	}
}

// loadRuleSetFiles gathers every rule set the configuration declares: a
// local one by its path, a remote one from the cache. A set neither yields is
// simply absent, and the matcher reports it unchecked.
func loadRuleSetFiles(s *store.Store) (*ruleSetFiles, error) {
	files := &ruleSetFiles{path: map[string]string{}, format: map[string]string{}}
	remote := map[string]bool{}
	if s.SingBox.Route != nil {
		for _, raw := range s.SingBox.Route.RuleSet {
			var set struct {
				Tag    string `json:"tag"`
				Type   string `json:"type"`
				Format string `json:"format"`
				Path   string `json:"path"`
			}
			if json.Unmarshal(raw, &set) != nil || set.Tag == "" {
				continue
			}
			files.format[set.Tag] = set.Format
			switch set.Type {
			case "local":
				files.path[set.Tag] = set.Path
			case "remote":
				remote[set.Tag] = true
			}
		}
	}
	if len(remote) == 0 {
		return files, nil
	}
	cachePath, cacheID, enabled := cacheFileOptions(s)
	if !enabled {
		return files, nil
	}
	dir, err := os.MkdirTemp("", "gproxy-rule-sets-")
	if err != nil {
		return nil, err
	}
	files.dir = dir
	// A copy, because sing-box holds the cache open with an exclusive lock
	// and a reader would wait on it. bbolt keeps two meta pages and opens the
	// newer valid one, so a copy taken mid-write still opens.
	copied := filepath.Join(dir, "cache.db")
	switch err := copyBounded(cachePath, copied, maxCacheFile); {
	case errors.Is(err, os.ErrNotExist):
		return files, nil
	case err != nil:
		files.Close()
		return nil, fmt.Errorf("read sing-box cache: %w", err)
	}
	db, err := bolt.Open(copied, 0o600, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		files.Close()
		return nil, fmt.Errorf("open sing-box cache: %w", err)
	}
	defer db.Close()
	written := 0
	err = db.View(func(tx *bolt.Tx) error {
		// sing-box nests its buckets under the cache ID, prefixed with a zero
		// byte, when one is configured.
		var bucket *bolt.Bucket
		if cacheID == "" {
			bucket = tx.Bucket([]byte("rule_set"))
		} else if root := tx.Bucket(append([]byte{0}, cacheID...)); root != nil {
			bucket = root.Bucket([]byte("rule_set"))
		}
		if bucket == nil {
			return nil
		}
		for tag := range remote {
			content, ok := savedContent(bucket.Get([]byte(tag)))
			if !ok {
				continue
			}
			written++
			path := filepath.Join(dir, strconv.Itoa(written)+".rule-set")
			if err := os.WriteFile(path, content, 0o600); err != nil {
				return err
			}
			files.path[tag] = path
		}
		return nil
	})
	if err != nil {
		files.Close()
		return nil, fmt.Errorf("read sing-box cache: %w", err)
	}
	return files, nil
}

// cacheFileOptions reads experimental.cache_file, with sing-box's defaults.
func cacheFileOptions(s *store.Store) (path, cacheID string, enabled bool) {
	var experimental struct {
		CacheFile struct {
			Enabled bool   `json:"enabled"`
			Path    string `json:"path"`
			CacheID string `json:"cache_id"`
		} `json:"cache_file"`
	}
	if len(s.SingBox.Experimental) == 0 || json.Unmarshal(s.SingBox.Experimental, &experimental) != nil {
		return "", "", false
	}
	path = experimental.CacheFile.Path
	if path == "" {
		path = config.SingBoxCache
	}
	return path, experimental.CacheFile.CacheID, experimental.CacheFile.Enabled
}

// savedContent decodes sing-box's SavedBinary: a version byte, then the
// rule set as a length-prefixed byte string. The fields after it, the update
// time and etag, are not needed here.
func savedContent(value []byte) ([]byte, bool) {
	reader := bytes.NewReader(value)
	if _, err := reader.ReadByte(); err != nil {
		return nil, false
	}
	length, err := binary.ReadUvarint(reader)
	if err != nil || length == 0 || length > uint64(reader.Len()) {
		return nil, false
	}
	content := make([]byte, length)
	if _, err := io.ReadFull(reader, content); err != nil {
		return nil, false
	}
	return content, true
}

func copyBounded(from, to string, limit int64) error {
	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	written, err := io.Copy(target, io.LimitReader(source, limit+1))
	if closeErr := target.Close(); err == nil {
		err = closeErr
	}
	if err == nil && written > limit {
		err = fmt.Errorf("larger than %d MiB", limit>>20)
	}
	return err
}

// matcher answers routing.Evaluate's question for one target with
// `sing-box rule-set match`, two sets at a time, in the order the rule lists
// them, so the set reported when several hold the target is always the same
// one.
func (f *ruleSetFiles) matcher(ctx context.Context, target string) func([]string) (string, []string, error) {
	return func(tags []string) (string, []string, error) {
		var unchecked, checkable []string
		for _, tag := range tags {
			if _, ok := f.path[tag]; ok {
				checkable = append(checkable, tag)
			} else {
				unchecked = append(unchecked, tag)
			}
		}
		for start := 0; start < len(checkable); start += 2 {
			batch := checkable[start:min(start+2, len(checkable))]
			found := make([]bool, len(batch))
			failures := make([]error, len(batch))
			done := make(chan int, len(batch))
			for i, tag := range batch {
				go func() {
					found[i], failures[i] = f.contains(ctx, tag, target)
					done <- i
				}()
			}
			for range batch {
				<-done
			}
			for i, tag := range batch {
				if failures[i] != nil {
					return "", nil, failures[i]
				}
				if found[i] {
					return tag, unchecked, nil
				}
			}
		}
		return "", unchecked, nil
	}
}

// contains runs one match. sing-box answers on stderr: a "match rules.[n]"
// line when the set holds the target, nothing when it does not, and exits 0
// either way; any other exit is a set it could not read.
func (f *ruleSetFiles) contains(ctx context.Context, tag, target string) (bool, error) {
	format := f.format[tag]
	if format == "" {
		format = "binary"
	}
	var output bytes.Buffer
	cmd := exec.CommandContext(ctx, config.SingBoxBin, "rule-set", "match", "-f", format, f.path[tag], target)
	cmd.Stdout = &cappedWriter{buffer: &output, limit: 4096}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		detail := strings.TrimSpace(output.String())
		if detail == "" {
			detail = err.Error()
		}
		return false, fmt.Errorf("sing-box rule-set match %s: %s", tag, firstLine(detail))
	}
	for _, line := range strings.Split(output.String(), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "match ") {
			return true, nil
		}
	}
	return false, nil
}

// cappedWriter keeps the first limit bytes and discards the rest, so a
// misbehaving subprocess cannot grow the buffer without bound.
type cappedWriter struct {
	buffer *bytes.Buffer
	limit  int
}

func (w *cappedWriter) Write(data []byte) (int, error) {
	if room := w.limit - w.buffer.Len(); room > 0 {
		w.buffer.Write(data[:min(room, len(data))])
	}
	return len(data), nil
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}
