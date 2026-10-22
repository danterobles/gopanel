package customers

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestNormalizeCustomerInput(t *testing.T) {
	value, err := Validate(Input{Name: "  Acme México  ", Email: " USER@EXAMPLE.TEST ", Phone: " +52 (81) 1234-5678 ", Status: "inactive", Notes: " primera\r\nsegunda\rtercera\tcolumna "})
	if err != nil {
		t.Fatal(err)
	}
	if value.Name != "Acme México" || value.Email != "user@example.test" || value.Phone != "+52 (81) 1234-5678" || value.Notes != "primera\nsegunda\ntercera\tcolumna" {
		t.Fatalf("unexpected normalization: %#v", value)
	}
	optional, err := Validate(Input{Name: "Cliente", Status: "active", Email: " ", Phone: " ", Notes: "\r\n "})
	if err != nil {
		t.Fatal(err)
	}
	record := optional.record()
	if record.Email.Valid || record.Phone.Valid || record.Notes.Valid {
		t.Fatal("empty optionals must become SQL NULL")
	}
}
func TestCustomerValidationBoundariesAndUnsafeText(t *testing.T) {
	valid := Input{Name: strings.Repeat("á", 180), Phone: strings.Repeat("☎", 40), Email: strings.Repeat("a", 241) + "@example.test", Status: "active", Notes: strings.Repeat("😀", 5000)}
	if _, err := Validate(valid); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		field string
		input Input
	}{
		{"name", Input{Name: "  ", Status: "active"}}, {"name", Input{Name: strings.Repeat("á", 181), Status: "active"}},
		{"name", Input{Name: "a\x00b", Status: "active"}}, {"name", Input{Name: "a\xff", Status: "active"}},
		{"email", Input{Name: "a", Email: "Name <a@example.test>", Status: "active"}}, {"email", Input{Name: "a", Email: strings.Repeat("a", 242) + "@example.test", Status: "active"}},
		{"phone", Input{Name: "a", Phone: strings.Repeat("☎", 41), Status: "active"}}, {"phone", Input{Name: "a", Phone: "55\n123", Status: "active"}},
		{"notes", Input{Name: "a", Notes: strings.Repeat("á", 5001), Status: "active"}}, {"notes", Input{Name: "a", Notes: "a\x7fb", Status: "active"}},
		{"status", Input{Name: "a", Status: "owner"}}, {"status", Input{Name: "a"}},
	} {
		_, err := Validate(entry.input)
		var invalid *ValidationError
		if !errors.As(err, &invalid) || invalid.Fields[entry.field] == "" {
			t.Errorf("%s validation missed: %v", entry.field, err)
		}
	}
}
func TestCustomerOptionsAndCanonicalURL(t *testing.T) {
	defaults, err := ParseOptions(url.Values{})
	if err != nil || defaults.Page != 1 || defaults.PageSize != 25 || defaults.Sort != "created_desc" {
		t.Fatal("bad defaults")
	}
	values := url.Values{"search": {" %_\\= & México "}, "status": {"inactive"}, "sort": {"name_desc"}, "page": {"3"}, "page_size": {"10"}}
	options, err := ParseOptions(values)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(ListURL(options, 4))
	if err != nil {
		t.Fatal(err)
	}
	again, err := ParseOptions(parsed.Query())
	if err != nil || again.Page != 4 || again.Search != "%_\\= & México" || again.Status != "inactive" || again.Sort != "name_desc" || again.PageSize != 10 {
		t.Fatal("URL did not preserve filters")
	}
	for _, values := range []url.Values{
		{"sort": {"name;DROP TABLE customers"}}, {"status": {"deleted"}}, {"page": {"0"}}, {"page": {"-1"}}, {"page": {"999999999999999999999999"}}, {"page": {"10001"}, "page_size": {"50"}},
		{"page_size": {"100"}}, {"page_size": {"-1"}}, {"page_size": {"0"}}, {"page_size": {"ten"}}, {"search": {strings.Repeat("á", 181)}}, {"search": {"x\x00y"}}, {"search": {"\xff"}},
	} {
		if _, err := ParseOptions(values); err == nil {
			t.Errorf("accepted invalid options %#v", values)
		}
	}
}
