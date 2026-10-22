package customers

import (
	"database/sql"
	"fmt"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	repo "gopanel/internal/modules/customers/repository"
)

// Input contains only editable business fields, never actor or tenant identifiers.
type Input struct{ Name, Email, Phone, Status, Notes string }
type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "invalid customer fields" }
func optional(s string) sql.NullString   { return sql.NullString{String: s, Valid: s != ""} }
func Validate(input Input) (Input, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Phone = strings.TrimSpace(input.Phone)
	input.Notes = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(input.Notes, "\r\n", "\n"), "\r", "\n"))
	fields := map[string]string{}
	for _, field := range []struct {
		name      string
		value     *string
		max       int
		multiline bool
	}{{"name", &input.Name, 180, false}, {"email", &input.Email, 254, false}, {"phone", &input.Phone, 40, false}, {"notes", &input.Notes, 5000, true}} {
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
	if input.Name == "" {
		fields["name"] = "Ingresa el nombre del cliente."
	}
	if input.Email != "" {
		address, err := mail.ParseAddress(input.Email)
		if err != nil || address.Address != input.Email || len(input.Email) > 254 {
			fields["email"] = "Ingresa un correo válido de hasta 254 bytes, sin nombre de presentación."
		}
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
	return repo.Input{Name: i.Name, Email: optional(i.Email), Phone: optional(i.Phone), Status: i.Status, Notes: optional(i.Notes)}
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
	case "created_desc", "created_asc", "name_asc", "name_desc":
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
	return "/app/customers?" + values.Encode()
}
