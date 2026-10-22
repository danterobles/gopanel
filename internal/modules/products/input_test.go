package products

import (
	"errors"
	"math"
	"net/url"
	"strings"
	"testing"
)

func TestExactMXNPrice(t *testing.T) {
	for _, c := range []struct {
		text  string
		cents int64
	}{{"0", 0}, {"0.01", 1}, {"001.2", 120}, {" 1250.50 ", 125050}, {"92233720368547758.07", math.MaxInt64}} {
		v, e := ParsePrice(c.text)
		if e != nil || v != c.cents {
			t.Errorf("%q: %d %v", c.text, v, e)
		}
		r, e := ParsePrice(FormatPrice(c.cents))
		if e != nil || r != c.cents {
			t.Fatal("price lost precision on roundtrip")
		}
	}
	for _, v := range []string{"", "-1", "+1", "1,25", "1,000.00", "1e2", "1.001", "1.", ".5", "NaN", "Infinity", "92233720368547758.08", "92233720368547759", "18446744073709551616", "１.00"} {
		if _, e := ParsePrice(v); e == nil {
			t.Errorf("invalid price accepted: %q", v)
		}
	}
}
func TestProductValidation(t *testing.T) {
	v, e := Validate(Input{SKU: "  SKÚ Uno  ", Name: "  Producto ", Description: " uno\r\ndos ", Price: "1.2", Currency: "MXN", Status: "active"})
	if e != nil || v.SKU != "SKÚ Uno" || v.Description != "uno\ndos" || v.Price != "1.20" || v.record().PriceCents != 120 {
		t.Fatalf("normalization: %#v %v", v, e)
	}
	base := Input{SKU: strings.Repeat("á", 80), Name: strings.Repeat("á", 180), Description: strings.Repeat("😀", 5000), Price: "0", Currency: "MXN", Status: "inactive"}
	if _, e = Validate(base); e != nil {
		t.Fatal(e)
	}
	for _, field := range []string{"sku", "name", "description", "price", "currency", "status"} {
		c := base
		switch field {
		case "sku":
			c.SKU = "a\x00"
		case "name":
			c.Name = " "
		case "description":
			c.Description = "\xff"
		case "price":
			c.Price = "1.001"
		case "currency":
			c.Currency = "USD"
		case "status":
			c.Status = "owner"
		}
		_, e = Validate(c)
		var invalid *ValidationError
		if !errors.As(e, &invalid) || invalid.Fields[field] == "" {
			t.Errorf("%s validation missing", field)
		}
	}
	base.SKU = strings.Repeat("á", 81)
	_, e = Validate(base)
	if e == nil {
		t.Fatal("SKU too long accepted")
	}
	base.SKU = "ok"
	base.Description = " "
	v, e = Validate(base)
	if e != nil || v.record().Description.Valid {
		t.Fatal("blank description must be NULL")
	}
}
func TestProductOptions(t *testing.T) {
	for _, sort := range []string{"name_asc", "name_desc", "price_asc", "price_desc", "created_asc", "created_desc"} {
		o, e := ParseOptions(url.Values{"sort": {sort}, "search": {" %_\\= "}, "page_size": {"50"}, "status": {"inactive"}})
		if e != nil {
			t.Fatal(e)
		}
		u, _ := url.Parse(ListURL(o, 2))
		r, e := ParseOptions(u.Query())
		if e != nil || r.Page != 2 || r.Search != "%_\\=" || r.Sort != sort || r.PageSize != 50 {
			t.Fatal("options lost in URL")
		}
	}
	for _, q := range []url.Values{{"sort": {"price;DROP"}}, {"page": {"0"}}, {"page_size": {"100"}}, {"page": {"10001"}, "page_size": {"50"}}, {"search": {"\xff"}}, {"status": {"deleted"}}} {
		if _, e := ParseOptions(q); e == nil {
			t.Fatal("invalid query accepted")
		}
	}
}
