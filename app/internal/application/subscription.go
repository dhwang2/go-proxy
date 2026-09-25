package application

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"go-proxy/internal/derived"
	"go-proxy/internal/subscription"
	"go-proxy/internal/user"
)

type SubscriptionOptions struct {
	User   string
	Node   string
	Target string
	Format subscription.Format
	JSON   bool
}

func (a *App) Subscription(ctx context.Context, p SubscriptionOptions) (Result, error) {
	snapshot, err := a.Snapshot(ctx)
	if err != nil {
		return Result{}, err
	}
	if p.User != "" && !user.Exists(snapshot.Store, p.User) {
		return Result{}, Invalid("user not found")
	}
	memberships := derived.Membership(snapshot.Store)
	names := make([]string, 0, len(memberships))
	capacity := 0
	for name := range memberships {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if p.User == "" || name == p.User {
			names = append(names, name)
			capacity += len(memberships[name])
		}
	}
	entries := make([]derived.MembershipEntry, 0, capacity)
	sort.Strings(names)
	for _, name := range names {
		members := memberships[name]
		sort.Slice(members, func(i, j int) bool { return members[i].Tag < members[j].Tag })
		for _, entry := range members {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
			if p.Node == "" || entry.Tag == p.Node {
				entries = append(entries, entry)
			}
		}
	}
	if p.Node != "" && len(entries) == 0 {
		return Result{}, Invalid("selected node has no matching membership")
	}
	renderer := subscription.NewRenderer(snapshot.Store, snapshot.Bindings, "", nil)
	// Surge and mihomo take a list of proxies, so both families are worth
	// emitting: a client can hold an entry for each and pick between them.
	resolveFamilies := p.Format == subscription.FormatSurge || p.Format == subscription.FormatMihomo || p.Format == ""
	// A format the client cannot represent is a property of the node, not a
	// failure of the request: a user whose nodes do not all share one format
	// gets the ones that do, and is told on stderr which were left out.
	// Refusing the whole export sent a Surge reader away with nothing when
	// most of their nodes were exportable.
	var skipped []string
	if p.Format != "" {
		kept := entries[:0]
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
			allowed := false
			for _, f := range renderer.Formats(entry) {
				if f == p.Format {
					allowed = true
				}
			}
			if allowed {
				kept = append(kept, entry)
				continue
			}
			skipped = append(skipped, entry.Tag)
		}
		// --node names one node and one format: answering that with an empty
		// export answers a different question, so it still fails.
		if len(skipped) > 0 && p.Node != "" {
			return Result{}, &Error{Code: "unsupported_format", Message: fmt.Sprintf("%s export unsupported for nodes: %s", p.Format, strings.Join(skipped, ", ")), Stage: "render"}
		}
		entries = kept
		if len(skipped) > 0 {
			noun := "nodes"
			if len(skipped) == 1 {
				noun = "node"
			}
			a.Progress(fmt.Sprintf("skipped %d %s with no %s export: %s", len(skipped), noun, p.Format, strings.Join(skipped, ", ")))
		}
	}
	var host string
	// Non-nil so a user with no nodes exports an empty list rather than the
	// JSON literal null, which no client can read as "no targets".
	targets := []subscription.SurgeTarget{}
	if len(entries) > 0 {
		observeCtx, cancel := ObservationContext(ctx)
		host, targets, err = subscription.ResolveTargets(observeCtx, p.Target, resolveFamilies)
		cancel()
		if err != nil {
			return Result{}, err
		}
	}
	renderer.SetTargets(host, targets)
	// The machine export is encoded while it is rendered: keeping every node as a struct and
	// then marshalling that graph again holds the whole export twice at capacity scale.
	//
	// Non-nil from the start: an export with no nodes is empty output, and a nil
	// Raw is read by the entry point as "this command produced no export", which
	// sent it down the fallback path to print the nil result as null.
	raw := []byte{}
	if p.JSON {
		raw = append(raw, `{"nodes":[`...)
	}
	if p.Format == subscription.FormatMihomo {
		raw = append(raw, "proxies:"...)
	}
	mihomoCount := 0
	var collected []SubscriptionLink
	var rendered []renderedFormat
	for index, entry := range entries {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		formats := renderer.Formats(entry)
		if len(formats) == 0 {
			return Result{}, fmt.Errorf("node %q has no supported export format", entry.Tag)
		}
		// Every format the node supports. Taking only the first showed one
		// client's spelling and hid the others, so a reader could not see that
		// a node they were looking at had a mihomo form at all.
		selected := formats
		if p.Format != "" {
			selected = []subscription.Format{p.Format}
		}
		// Reserve the remaining export from the average node size once the buffer fills: copying a
		// multi-megabyte buffer forward step by step costs more than rendering it.
		if index > 0 && cap(raw)-len(raw) < len(raw)/index {
			raw = slices.Grow(raw, len(raw)/index*(len(entries)-index)+1024)
		}
		if p.JSON {
			rendered = rendered[:0]
			port := entry.Port
			for _, format := range selected {
				links, renderErr := renderer.Render(ctx, entry, format)
				if renderErr != nil {
					return Result{}, renderErr
				}
				for _, link := range links {
					if err := ctx.Err(); err != nil {
						return Result{}, err
					}
					port = link.Port
				}
				if len(links) > 0 {
					rendered = append(rendered, renderedFormat{format: format, links: links})
				}
			}
			slices.SortFunc(rendered, func(a, b renderedFormat) int { return strings.Compare(string(a.format), string(b.format)) })
			if index > 0 {
				raw = append(raw, ',')
			}
			raw = append(raw, `{"tag":`...)
			raw = appendJSONString(raw, entry.Tag)
			raw = append(raw, `,"user":`...)
			raw = appendJSONString(raw, entry.UserName)
			raw = append(raw, `,"protocol":`...)
			raw = appendJSONString(raw, entry.Proto)
			raw = append(raw, `,"port":`...)
			raw = strconv.AppendInt(raw, int64(port), 10)
			raw = append(raw, `,"formats":[`...)
			for i, format := range formats {
				if i > 0 {
					raw = append(raw, ',')
				}
				raw = appendJSONString(raw, string(format))
			}
			raw = append(raw, `],"content":{`...)
			for i, item := range rendered {
				if i > 0 {
					raw = append(raw, ',')
				}
				raw = appendJSONString(raw, string(item.format))
				raw = append(raw, ':', '[')
				for j, link := range item.links {
					if j > 0 {
						raw = append(raw, ',')
					}
					raw = appendJSONString(raw, link.Content)
				}
				raw = append(raw, ']')
			}
			raw = append(raw, `}}`...)
			continue
		}
		for _, format := range selected {
			links, renderErr := renderer.Render(ctx, entry, format)
			if renderErr != nil {
				return Result{}, renderErr
			}
			for _, link := range links {
				if err := ctx.Err(); err != nil {
					return Result{}, err
				}
				if p.Format == "" {
					// The default export is read, not pasted into a client, so
					// it is handed back structured and grouped for display
					// rather than as a flat stream with a header per line.
					collected = append(collected, SubscriptionLink{
						Format: string(format), User: link.UserName, Tag: link.Tag,
						Family: link.Family, Content: link.Content,
					})
					continue
				}
				if format == subscription.FormatMihomo {
					// Asked for by name, so this is the document a client
					// loads: a proxies key with sequence items under it. The
					// default view prints the same wrapper around its own
					// [mihomo] section, so either export can be used as it is.
					raw = append(raw, "\n  - "...)
					raw = append(raw, link.Content...)
					mihomoCount++
					continue
				}
				raw = append(raw, link.Content...)
				raw = append(raw, '\n')
			}
		}
	}
	if p.JSON {
		encoded, err := json.Marshal(targets)
		if err != nil {
			return Result{}, err
		}
		raw = append(raw, `],"targets":`...)
		raw = append(raw, encoded...)
		raw = append(raw, '}')
		return Result{Data: json.RawMessage(raw)}, nil
	}
	if p.Format == "" {
		// Grouped for the eye: a format at a time, a user at a time, and the
		// families together within a user so one block can be taken at once.
		slices.SortStableFunc(collected, func(a, b SubscriptionLink) int {
			for _, pair := range [][2]string{{a.Format, b.Format}, {a.User, b.User}, {a.Family, b.Family}, {a.Tag, b.Tag}} {
				if order := strings.Compare(pair[0], pair[1]); order != 0 {
					return order
				}
			}
			return 0
		})
		return Result{Data: map[string]any{"links": collected}}, nil
	}
	if p.Format == subscription.FormatMihomo {
		// An empty list is written as one, not as a key with nothing under it:
		// "proxies:" alone parses as null and mihomo rejects that.
		if mihomoCount == 0 {
			raw = append(raw, " []"...)
		}
		raw = append(raw, '\n')
	}
	return Result{Raw: raw}, nil
}

// SubscriptionLink is one client link with enough about it to be grouped:
// which format it is, whose it is, which node it reaches and over which
// address family.
type SubscriptionLink struct {
	Format  string `json:"format"`
	User    string `json:"user"`
	Tag     string `json:"tag"`
	Family  string `json:"family,omitempty"`
	Content string `json:"content"`
}

type renderedFormat struct {
	format subscription.Format
	links  []subscription.Link
}

// appendJSONString appends s as a JSON string literal, matching encoding/json including its
// escaping of <, > and &, so that the rendered export needs no second encoding pass.
func appendJSONString(dst []byte, s string) []byte {
	const hex = "0123456789abcdef"
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if b >= 0x20 && b != '"' && b != '\\' && b != '<' && b != '>' && b != '&' {
				i++
				continue
			}
			dst = append(dst, s[start:i]...)
			switch b {
			case '\\', '"':
				dst = append(dst, '\\', b)
			case '\b':
				dst = append(dst, '\\', 'b')
			case '\f':
				dst = append(dst, '\\', 'f')
			case '\n':
				dst = append(dst, '\\', 'n')
			case '\r':
				dst = append(dst, '\\', 'r')
			case '\t':
				dst = append(dst, '\\', 't')
			default:
				dst = append(dst, '\\', 'u', '0', '0', hex[b>>4], hex[b&0xF])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			dst = append(dst, s[start:i]...)
			dst = utf8.AppendRune(dst, utf8.RuneError)
			i += size
			start = i
			continue
		}
		// U+2028 and U+2029 are valid in JSON but break JSONP, so encoding/json escapes them.
		if r == '\u2028' || r == '\u2029' {
			dst = append(dst, s[start:i]...)
			dst = append(dst, '\\', 'u', '2', '0', '2', hex[r&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"')
}
