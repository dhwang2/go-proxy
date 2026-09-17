package core

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallBinaryFormatsAndAtomicFailure(t *testing.T) {
	payload := []byte("verified executable")
	for _, format := range []string{"raw", "zip", "tar.gz"} {
		t.Run(format, func(t *testing.T) {
			var data bytes.Buffer
			switch format {
			case "zip":
				z := zip.NewWriter(&data)
				w, err := z.Create("nested/program")
				if err != nil {
					t.Fatal(err)
				}
				w.Write(payload)
				if err := z.Close(); err != nil {
					t.Fatal(err)
				}
			case "tar.gz":
				gz := gzip.NewWriter(&data)
				tw := tar.NewWriter(gz)
				if err := tw.WriteHeader(&tar.Header{Name: "nested/program", Mode: 0755, Size: int64(len(payload))}); err != nil {
					t.Fatal(err)
				}
				tw.Write(payload)
				tw.Close()
				gz.Close()
			default:
				data.Write(payload)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data.Bytes()) }))
			defer server.Close()
			dest := filepath.Join(t.TempDir(), "program")
			os.WriteFile(dest, []byte("previous"), 0755)
			digest := fmt.Sprintf("%x", sha256.Sum256(data.Bytes()))
			validate := func(path string) error {
				got, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if !bytes.Equal(got, payload) {
					return fmt.Errorf("wrong executable")
				}
				return nil
			}
			if err := InstallBinary(context.Background(), server.URL+"/artifact."+format, "wrong", dest, "program", validate); err == nil {
				t.Fatal("accepted wrong checksum")
			}
			if got, _ := os.ReadFile(dest); string(got) != "previous" {
				t.Fatal("failed download replaced original")
			}
			if err := InstallBinary(context.Background(), server.URL+"/artifact."+format, digest, dest, "program", func(string) error { return fmt.Errorf("reject") }); err == nil {
				t.Fatal("ignored validator")
			}
			if got, _ := os.ReadFile(dest); string(got) != "previous" {
				t.Fatal("failed validation replaced original")
			}
			if err := InstallBinary(context.Background(), server.URL+"/artifact."+format, digest, dest, "program", validate); err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(dest); !bytes.Equal(got, payload) {
				t.Fatal("wrong installed content")
			}
			entries, _ := os.ReadDir(filepath.Dir(dest))
			if len(entries) != 1 {
				t.Fatal("staging files leaked")
			}
		})
	}
}

func TestInstallBinaryCancelledPreservesOriginal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	dest := filepath.Join(t.TempDir(), "program")
	os.WriteFile(dest, []byte("previous"), 0755)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := InstallBinary(ctx, server.URL, "x", dest, "program", nil); err == nil {
		t.Fatal("cancelled download succeeded")
	}
	if got, _ := os.ReadFile(dest); string(got) != "previous" {
		t.Fatal("cancelled download replaced original")
	}
	entries, _ := os.ReadDir(filepath.Dir(dest))
	if len(entries) != 1 {
		t.Fatal("staging files leaked")
	}
}
