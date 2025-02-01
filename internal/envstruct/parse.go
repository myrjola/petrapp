package envstruct

import (
	"github.com/myrjola/petrapp/internal/errors"
	"log/slog"
	"reflect"
)

var (
	ErrEnvNotSet    = errors.NewSentinel("environment variable not set")
	ErrInvalidValue = errors.NewSentinel("v must be a pointer to a struct")
)

// Populate populates the fields of the pointer to struct v with values from the environment.
//
// lookupEnv is used to look up environment variables. It has the same signature as [os.LookupEnv].
// Fields in the struct v must be tagged with `env:"ENV_VAR"` where ENV_VAR is the name of the environment variable.
// If no environment variable matching ENV_VAR is provided, the field must be tagged with default value
// `envDefault:"value"` or else ErrEnvNotSet is returned.
func Populate(v any, lookupEnv func(string) (string, bool)) error {
	ptrRef := reflect.ValueOf(v)
	if ptrRef.Kind() != reflect.Ptr {
		return errors.Wrap(ErrInvalidValue, "not pointer", slog.Any("v", v))
	}
	ref := ptrRef.Elem()
	if ref.Kind() != reflect.Struct {
		return errors.Wrap(ErrInvalidValue, "not struct", slog.Any("v", v))
	}

	refType := ref.Type()

	var (
		errorList  []error
		ok         bool
		envVarName string
	)

	for i := range refType.NumField() {
		refField := ref.Field(i)
		refTypeField := refType.Field(i)
		tag := refTypeField.Tag

		envVarName, ok = tag.Lookup("env")
		if ok {
			if !refField.CanSet() {
				errorList = append(errorList, errors.Wrap(ErrInvalidValue, "cannot set field",
					slog.String("fieldName", refTypeField.Name)))
				continue
			}

			if refField.Kind() != reflect.String {
				errorList = append(errorList, errors.Wrap(ErrInvalidValue, "only strings are supported",
					slog.String("envVarName", envVarName),
					slog.String("fieldType", refField.Kind().String()),
					slog.String("fieldName", refTypeField.Name),
				))
				continue
			}

			var (
				val string
				err error
			)
			if val, err = envLookupWithFallback(envVarName, tag, lookupEnv); err != nil {
				errorList = append(errorList, err)
				continue
			}

			refField.Set(reflect.ValueOf(val))
		}
	}

	if len(errorList) != 0 {
		// Join the errors into a single error.
		return errors.Join(errorList...)
	}

	return nil
}

func envLookupWithFallback(
	envVarName string, tag reflect.StructTag, lookupEnv func(string) (string, bool)) (string, error) {
	envVarValue, ok := lookupEnv(envVarName)
	if !ok {
		envVarValue, ok = tag.Lookup("envDefault")
		if !ok {
			return "", errors.Wrap(ErrEnvNotSet, "environment variable not set", slog.String("envVarName", envVarName))
		}
	}
	return envVarValue, nil
}
