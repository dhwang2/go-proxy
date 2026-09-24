package jsonorder

import "testing"

func TestRoundTripKeepsKeyOrderAndDoesNotEscapeHTML(t *testing.T) {
	input := `{"tag":"public4","type":"https","server":"8.8.8.8","server_port":443,"tls":{"enabled":true,"server_name":"dns.google"},"note":"<redacted>","list":[],"none":null}`
	value, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	out, _ := value.MarshalJSON()
	if string(out) != input {
		t.Fatalf("round trip:\n%s\nwant:\n%s", out, input)
	}
}

func TestReorderPutsNamedKeysFirstAndKeepsTheRest(t *testing.T) {
	value, err := Parse([]byte(`{"path":"/dns-query","extra":1,"tag":"a","type":"https","other":2}`))
	if err != nil {
		t.Fatal(err)
	}
	value.Reorder("tag", "type", "server", "path")
	out, _ := value.MarshalJSON()
	if want := `{"tag":"a","type":"https","path":"/dns-query","extra":1,"other":2}`; string(out) != want {
		t.Fatalf("reorder = %s, want %s", out, want)
	}
}

func TestSetReplacesInPlaceOrAppends(t *testing.T) {
	value, _ := Parse([]byte(`{"type":"direct","tag":"x"}`))
	value.Set("tag", String("direct"))
	value.Set("domain_resolver", String("public4"))
	out, _ := value.MarshalJSON()
	if want := `{"type":"direct","tag":"direct","domain_resolver":"public4"}`; string(out) != want {
		t.Fatalf("set = %s, want %s", out, want)
	}
}
