// Package validation translates request tags into field errors for the frontend.
package validation

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
	"github.com/haidarrasyad/absensi-api/internal/shared"
)

var phoneCharacters = regexp.MustCompile(`^[+0-9() .-]+$`)
var requests = newValidator()

func newValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		name := strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	_ = v.RegisterValidation("notblank", func(f validator.FieldLevel) bool { return strings.TrimSpace(f.Field().String()) != "" })
	_ = v.RegisterValidation("maxbytes", func(f validator.FieldLevel) bool {
		n, err := strconv.Atoi(f.Param())
		return err == nil && len(f.Field().String()) <= n
	})
	_ = v.RegisterValidation("phone", func(f validator.FieldLevel) bool {
		raw := f.Field().String()
		normalized := shared.NormalizePhone(raw)
		return phoneCharacters.MatchString(raw) && strings.HasPrefix(normalized, "628") && len(normalized) >= 10 && len(normalized) <= 16
	})
	_ = v.RegisterValidation("nik", func(f validator.FieldLevel) bool { return shared.ValidNIK(f.Field().String()) })
	return v
}

// Struct reports every invalid field using its JSON key. Request values are never
// included in messages, so passwords/phone numbers cannot leak through errors.
func Struct(request any) error {
	if normalizer, ok := request.(interface{ Normalize() }); ok {
		normalizer.Normalize()
	}
	err := requests.Struct(request)
	if err == nil {
		return nil
	}
	var failures validator.ValidationErrors
	if !errors.As(err, &failures) {
		return err
	}
	fields := make(map[string]string, len(failures))
	typ := reflect.TypeOf(request)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	for _, failure := range failures {
		key := failure.Namespace()
		if _, rest, found := strings.Cut(key, "."); found {
			key = rest
		}
		label := failure.Field()
		if f, ok := typ.FieldByName(failure.StructField()); ok && f.Tag.Get("label") != "" {
			label = f.Tag.Get("label")
		}
		switch failure.Tag() {
		case "required", "notblank":
			fields[key] = label + " Wajib Diisi"
		case "email":
			fields[key] = "Format Email Tidak Valid"
		case "nik":
			fields[key] = "NIK Harus 16 Digit Angka"
		case "phone":
			fields[key] = "Format Nomor HP Tidak Valid"
		case "maxbytes":
			fields[key] = fmt.Sprintf("%s Maksimal %s Byte", label, failure.Param())
		case "min", "max":
			bound := "Minimal"
			if failure.Tag() == "max" {
				bound = "Maksimal"
			}
			unit := "Karakter"
			if failure.Kind() == reflect.Slice || failure.Kind() == reflect.Array {
				unit = "Item"
			}
			fields[key] = fmt.Sprintf("%s %s %s %s", label, bound, failure.Param(), unit)
		default:
			fields[key] = label + " Tidak Valid"
		}
	}
	return apperror.Validation(fields)
}
