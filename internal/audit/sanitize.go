package audit

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// DefaultRedactFields lists structured field/key names that must never
// appear in plaintext in an audit record or log line. This is the
// hard-coded floor; config.AuditConfig.RedactFields (from the operator's
// YAML) is additive to this list, never a replacement for it.
var DefaultRedactFields = []string{
	"client_private_key",
	"client_cert",
	"ca_certificate",
	"api_key",
	"approval_token",
	"password",
	"secret",
}

const redactedPlaceholder = "[REDACTED]"

// Sanitizer redacts sensitive values out of free-form maps, slices, and
// strings before they are turned into an audit Event or written to any
// log output.
//
// The redaction strategy is two-pronged:
//
//  1. Key-name based: any map key whose lowercased name matches a
//     configured redactable field name is replaced wholesale with the
//     redacted placeholder. This catches the common case (an explicit
//     "client_private_key" field in an error payload).
//
//  2. Content based: any string value containing a PEM-shaped
//     "-----BEGIN ... -----END ...-----" block is rewritten with the
//     block excised. This catches secrets embedded under innocent key
//     names (e.g. an error message that interpolates a TLS error
//     containing PEM bytes).
//
// Both passes are recursive across maps, slices, structs, and pointers
// so a secret embedded at any depth is still redacted.
type Sanitizer struct {
	redactKeys map[string]struct{}
}

// NewSanitizer builds a Sanitizer from the default redaction list plus
// any operator-configured extra field names.
func NewSanitizer(extra []string) *Sanitizer {
	keys := make(map[string]struct{}, len(DefaultRedactFields)+len(extra))
	for _, k := range DefaultRedactFields {
		keys[normalizeFieldName(k)] = struct{}{}
	}
	for _, k := range extra {
		keys[normalizeFieldName(k)] = struct{}{}
	}
	return &Sanitizer{redactKeys: keys}
}

// normalizeFieldName folds a field name to a form that matches
// regardless of naming convention: DefaultRedactFields is written
// snake_case ("client_private_key"), but callers may hand SanitizeAny a
// Go struct whose field name is CamelCase ("ClientPrivateKey") with no
// json tag, or a map key in yet another convention. Lowercasing alone
// does not bridge that gap ("clientprivatekey" vs "client_private_key"),
// so underscores/hyphens are stripped too before comparing.
func normalizeFieldName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, "-", "")
	return name
}

// ShouldRedact reports whether a field name matches the redaction list.
func (s *Sanitizer) ShouldRedact(fieldName string) bool {
	_, ok := s.redactKeys[normalizeFieldName(fieldName)]
	return ok
}

// SanitizeMap returns a copy of m with any key matching the redaction
// list replaced by a fixed placeholder. String values are also passed
// through SanitizeString so any embedded PEM block is excised even
// under an innocent key name. (Note: this is the shallow-string-map
// case; for arbitrary nested structures use SanitizeAny.)
func (s *Sanitizer) SanitizeMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if s.ShouldRedact(k) {
			out[k] = redactedPlaceholder
			continue
		}
		out[k] = s.SanitizeString(v)
	}
	return out
}

// SanitizeAny walks v recursively and returns a redacted copy. Maps,
// slices, structs, and pointers are descended into; strings are passed
// through SanitizeString; everything else is returned as-is. This is
// the single choke point for redacting any value before it is embedded
// in an audit Event.
//
// Struct redaction uses the struct's JSON tags (matching the wire
// shape); a field tagged `json:"client_private_key"` or named
// ClientPrivateKey is redacted by either key.
func (s *Sanitizer) SanitizeAny(v any) any {
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map:
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			ks := fmt.Sprint(iter.Key().Interface())
			if s.ShouldRedact(ks) {
				out[ks] = redactedPlaceholder
				continue
			}
			out[ks] = s.SanitizeAny(iter.Value().Interface())
		}
		return out
	case reflect.Slice, reflect.Array:
		out := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = s.SanitizeAny(rv.Index(i).Interface())
		}
		return out
	case reflect.Ptr, reflect.Interface:
		if rv.IsNil() {
			return nil
		}
		return s.SanitizeAny(rv.Elem().Interface())
	case reflect.Struct:
		return s.sanitizeStruct(rv)
	case reflect.String:
		return s.SanitizeString(rv.String())
	default:
		return v
	}
}

// sanitizeStruct redacts a struct field by field, honoring both the Go
// field name and any `json` tag (matching the wire shape, so a struct
// serialized to JSON and then re-parsed would redact identically).
func (s *Sanitizer) sanitizeStruct(rv reflect.Value) any {
	rt := rv.Type()
	out := make(map[string]any, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}
		// Honor json tag name if present, else Go field name.
		name := field.Name
		if tag := field.Tag.Get("json"); tag != "" {
			if comma := strings.Index(tag, ","); comma >= 0 {
				tag = tag[:comma]
			}
			if tag != "" && tag != "-" {
				name = tag
			}
		}
		if s.ShouldRedact(name) || s.ShouldRedact(field.Name) {
			out[name] = redactedPlaceholder
			continue
		}
		out[name] = s.SanitizeAny(rv.Field(i).Interface())
	}
	return out
}

// SanitizeString redacts any PEM-shaped substring inside s. Defense in
// depth on top of key-name based redaction: an error string or VQL row
// can carry a leaked PEM block under an innocent key name, and a
// future-proof log shipper may not know which keys are sensitive. This
// pass strips every "-----BEGIN ... -----END ...-----\n" block.
func (s *Sanitizer) SanitizeString(v string) string {
	for {
		idx := strings.Index(v, "-----BEGIN")
		if idx == -1 {
			return v
		}
		end := strings.Index(v[idx:], "-----END")
		if end == -1 {
			// Truncated PEM block — strip from BEGIN to end of string.
			return v[:idx] + redactedPlaceholder
		}
		// Strip through the end of the -----END ... ----- line,
		// including the trailing newline if present.
		relEnd := idx + end
		nl := strings.IndexByte(v[relEnd:], '\n')
		if nl == -1 {
			return v[:idx] + redactedPlaceholder
		}
		v = v[:idx] + redactedPlaceholder + v[relEnd+nl+1:]
	}
}

// SanitizeJSONString returns the JSON-encoded form of v with every
// redactable field and every embedded PEM block redacted. Tool handlers
// should use this when stuffing an arbitrary payload into an audit
// Event.Reason or other free-form string field.
func (s *Sanitizer) SanitizeJSONString(v any) string {
	sanitized := s.SanitizeAny(v)
	b, err := json.Marshal(sanitized)
	if err != nil {
		return redactedPlaceholder
	}
	return string(b)
}
