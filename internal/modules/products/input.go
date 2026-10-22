package products

import (
	"database/sql"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	repo "gopanel/internal/modules/products/repository"
)

type Input struct {
	SKU, Name, Description, Price, Currency, Status string
	priceCents                                      int64
}
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "invalid product fields" }
func FormatPrice(cents int64) string     { return fmt.Sprintf("%d.%02d", cents/100, cents%100) }

// ParsePrice never uses floating point and rejects precision loss or BIGINT overflow.
func ParsePrice(value string) (int64, error) {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, fmt.Errorf("invalid MXN amount")
	}
	for _, part := range parts {
		if part == "" {
			return 0, fmt.Errorf("invalid MXN amount")
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return 0, fmt.Errorf("invalid MXN amount")
			}
		}
	}
	fraction := uint64(0)
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			return 0, fmt.Errorf("invalid MXN precision")
		}
		fraction, _ = strconv.ParseUint(parts[1], 10, 64)
		if len(parts[1]) == 1 {
			fraction *= 10
		}
	}
	whole, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil || whole > uint64(math.MaxInt64)/100 || (whole == uint64(math.MaxInt64)/100 && fraction > uint64(math.MaxInt64)%100) {
		return 0, fmt.Errorf("MXN amount out of range")
	}
	return int64(whole*100 + fraction), nil
}
func Validate(input Input) (Input, error) {
	input.SKU = strings.TrimSpace(input.SKU)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(input.Description, "\r\n", "\n"), "\r", "\n"))
	input.Price = strings.TrimSpace(input.Price)
	fields := map[string]string{}
	for _, field := range []struct {
		name      string
		value     *string
		max       int
		multiline bool
	}{{"sku", &input.SKU, 80, false}, {"name", &input.Name, 180, false}, {"description", &input.Description, 5000, true}} {
		if !utf8.ValidString(*field.value) {
			fields[field.name] = "Usa texto UTF-8 válido."
			*field.value = strings.ToValidUTF8(*field.value, "�")
		}
		if utf8.RuneCountInString(*field.value) > field.max {
			fields[field.name] = fmt.Sprintf("Máximo %d caracteres.", field.max)
		}
		for _, r := range *field.value {
			if unicode.IsControl(r) && !(field.multiline && (r == '\n' || r == '\t')) {
				fields[field.name] = "El campo contiene caracteres de control no permitidos."
				break
			}
		}
	}
	if input.SKU == "" {
		fields["sku"] = "Ingresa el SKU."
	}
	if input.Name == "" {
		fields["name"] = "Ingresa el nombre del producto."
	}
	cents, err := ParsePrice(input.Price)
	if err != nil {
		fields["price"] = "Ingresa un precio no negativo con punto y hasta dos decimales, sin miles ni exponentes."
	} else {
		input.priceCents = cents
		input.Price = FormatPrice(cents)
	}
	if input.Currency != "MXN" {
		fields["currency"] = "La moneda debe ser MXN."
	}
	if input.Status != "active" && input.Status != "inactive" {
		fields["status"] = "Selecciona activo o inactivo."
	}
	if len(fields) > 0 {
		return input, &ValidationError{Fields: fields}
	}
	return input, nil
}
func (i Input) record() repo.Input {
	return repo.Input{Sku: i.SKU, Name: i.Name, Description: sql.NullString{String: i.Description, Valid: i.Description != ""}, PriceCents: i.priceCents, Currency: i.Currency, Status: i.Status}
}

func ParseOptions(values url.Values) (repo.ListOptions, error) {
	out := repo.ListOptions{Search: strings.TrimSpace(values.Get("search")), Status: values.Get("status"), Sort: values.Get("sort"), Page: 1, PageSize: 25}
	if out.Sort == "" {
		out.Sort = "created_desc"
	}
	if !utf8.ValidString(out.Search) || utf8.RuneCountInString(out.Search) > 180 {
		return out, repo.ErrInvalidOptions
	}
	for _, r := range out.Search {
		if unicode.IsControl(r) {
			return out, repo.ErrInvalidOptions
		}
	}
	if out.Status != "" && out.Status != "active" && out.Status != "inactive" {
		return out, repo.ErrInvalidOptions
	}
	switch out.Sort {
	case "created_desc", "created_asc", "name_asc", "name_desc", "price_asc", "price_desc":
	default:
		return out, repo.ErrInvalidOptions
	}
	if v := values.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 10001 {
			return out, repo.ErrInvalidOptions
		}
		out.Page = n
	}
	if v := values.Get("page_size"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || (n != 10 && n != 25 && n != 50) {
			return out, repo.ErrInvalidOptions
		}
		out.PageSize = n
	}
	if (out.Page-1)*out.PageSize > 100000 {
		return out, repo.ErrInvalidOptions
	}
	return out, nil
}
func ListURL(options repo.ListOptions, page int) string {
	values := url.Values{"search": {options.Search}, "status": {options.Status}, "sort": {options.Sort}, "page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(options.PageSize)}}
	return "/app/products?" + values.Encode()
}
