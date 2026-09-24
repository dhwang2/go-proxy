package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"go-proxy/pkg/jsonorder"
)

// The colours `jq -C` uses, which is how shell-proxy printed a configuration.
// They are the terminal's own palette rather than truecolor, so the output
// matches whatever theme the reader's jq output already has.
const (
	jqKey    = "\x1b[34;1m"
	jqString = "\x1b[0;32m"
	jqPlain  = "\x1b[0;39m" // numbers, true, false
	jqNull   = "\x1b[1;30m"
	jqPunct  = "\x1b[1;39m" // brackets, braces, colons and commas
)

// renderConfig prints the configuration itself, without the envelope the
// JSON result carries, in shell-proxy's layout: two-space indentation, keys in
// the order the file holds them, each DNS and route rule on one line, and the
// route's rule_set catalogue folded to "..." -- it is dozens of download URLs
// that are in the real file but tell a reader nothing.
func renderConfig(w io.Writer, p palette, fields map[string]any) bool {
	configuration, ok := fields["configuration"]
	if !ok {
		return false
	}
	raw, err := json.Marshal(configuration)
	if err != nil {
		return false
	}
	value, err := jsonorder.Parse(raw)
	if err != nil {
		return false
	}
	compact := map[*jsonorder.Value]bool{}
	if component, _ := fields["component"].(string); component == "sing-box" {
		for _, section := range []string{"dns", "route"} {
			if rules := value.Get(section).Get("rules"); rules != nil && rules.Kind == jsonorder.Array {
				for _, rule := range rules.Items {
					compact[rule] = true
				}
			}
		}
		if route := value.Get("route"); route != nil && route.Get("rule_set") != nil {
			route.Set("rule_set", &jsonorder.Value{Kind: jsonorder.Array, Items: []*jsonorder.Value{jsonorder.String("...")}})
		}
	}
	printer := configPrinter{p: p, compact: compact}
	printer.value(value, 0)
	printer.buf.WriteByte('\n')
	_, _ = w.Write(printer.buf.Bytes())
	return true
}

type configPrinter struct {
	p       palette
	compact map[*jsonorder.Value]bool
	buf     bytes.Buffer
}

func (c *configPrinter) colour(code, text string) {
	if c.p.on {
		c.buf.WriteString(code + text + ansiReset)
		return
	}
	c.buf.WriteString(text)
}

func (c *configPrinter) value(v *jsonorder.Value, depth int) {
	if c.compact[v] {
		c.flat(v)
		return
	}
	switch v.Kind {
	case jsonorder.Object:
		if len(v.Keys) == 0 {
			c.colour(jqPunct, "{}")
			return
		}
		c.colour(jqPunct, "{")
		for index, key := range v.Keys {
			c.newline(depth + 1)
			c.key(key)
			c.colour(jqPunct, ":")
			c.buf.WriteByte(' ')
			c.value(v.Fields[index], depth+1)
			if index < len(v.Keys)-1 {
				c.colour(jqPunct, ",")
			}
		}
		c.newline(depth)
		c.colour(jqPunct, "}")
	case jsonorder.Array:
		if len(v.Items) == 0 {
			c.colour(jqPunct, "[]")
			return
		}
		c.colour(jqPunct, "[")
		for index, item := range v.Items {
			c.newline(depth + 1)
			c.value(item, depth+1)
			if index < len(v.Items)-1 {
				c.colour(jqPunct, ",")
			}
		}
		c.newline(depth)
		c.colour(jqPunct, "]")
	default:
		c.scalar(v.Raw)
	}
}

// flat prints a value on one line with no spaces, as `jq -c` does.
func (c *configPrinter) flat(v *jsonorder.Value) {
	switch v.Kind {
	case jsonorder.Object:
		c.colour(jqPunct, "{")
		for index, key := range v.Keys {
			if index > 0 {
				c.colour(jqPunct, ",")
			}
			c.key(key)
			c.colour(jqPunct, ":")
			c.flat(v.Fields[index])
		}
		c.colour(jqPunct, "}")
	case jsonorder.Array:
		c.colour(jqPunct, "[")
		for index, item := range v.Items {
			if index > 0 {
				c.colour(jqPunct, ",")
			}
			c.flat(item)
		}
		c.colour(jqPunct, "]")
	default:
		c.scalar(v.Raw)
	}
}

func (c *configPrinter) key(name string) {
	encoded, _ := jsonorder.String(name).MarshalJSON()
	c.colour(jqKey, string(encoded))
}

func (c *configPrinter) scalar(raw json.RawMessage) {
	text := string(raw)
	switch {
	case strings.HasPrefix(text, `"`):
		c.colour(jqString, text)
	case text == "null":
		c.colour(jqNull, text)
	default:
		c.colour(jqPlain, text)
	}
}

func (c *configPrinter) newline(depth int) {
	c.buf.WriteByte('\n')
	c.buf.WriteString(strings.Repeat("  ", depth))
}
