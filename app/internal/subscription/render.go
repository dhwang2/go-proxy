package subscription

import (
	"go-proxy/internal/derived"
	"go-proxy/internal/store"
)

// Format represents a subscription output format.
type Format string

const (
	FormatSurge   Format = "surge"
	FormatSingBox Format = "singbox"
	FormatURI     Format = "uri"
)

// Link holds a generated subscription link or config for one protocol membership.
type Link struct {
	Proto    string
	Tag      string
	Port     int
	UserName string
	Content  string // the generated link or config snippet
}

// Render generates subscription links for a user in the specified format.
func Render(s *store.Store, userName string, format Format, host string) []Link {
	membership := derived.Membership(s)
	entries, ok := membership[userName]
	if !ok {
		return nil
	}

	if host == "" {
		host = DetectTarget()
	}

	var links []Link
	for _, entry := range entries {
		ib := derived.FindInbound(s, entry.Tag)
		if ib == nil {
			continue
		}
		var content string
		switch format {
		case FormatSurge:
			content = renderSurge(ib, entry, host)
		case FormatSingBox:
			content = renderSingBox(ib, entry, host)
		case FormatURI:
			content = renderURI(ib, entry, host)
		default:
			continue
		}
		links = append(links, Link{
			Proto:    entry.Proto,
			Tag:      entry.Tag,
			Port:     entry.Port,
			UserName: entry.UserName,
			Content:  content,
		})
	}
	return links
}
