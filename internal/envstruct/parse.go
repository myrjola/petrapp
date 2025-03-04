package envstruct

import (
	"errors"
	"fmt"
	"reflect"
)

var (
	ErrEnvNotSet    = errors.New("environment variable not set")
	ErrInvalidValue = errors.New("v must be a pointer to a struct")
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
		return fmt.Errorf("%w: not pointer: %v", ErrInvalidValue, v)
	}
	ref := ptrRef.Elem()
	if ref.Kind() != reflect.Struct {
		return fmt.Errorf("%w: not struct: %v", ErrInvalidValue, v)
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
				errorList = append(errorList, fmt.Errorf("%w: cannot set field: %s",
					ErrInvalidValue, refTypeField.Name))
				continue
			}

			if refField.Kind() != reflect.String {
				errorList = append(errorList, fmt.Errorf("%w: only strings are supported - field: %s, type: %s, env: %s",
					ErrInvalidValue, refTypeField.Name, refField.Kind().String(), envVarName))
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
			return "", fmt.Errorf("%w: environment variable not set: %s", ErrEnvNotSet, envVarName)
		}
	}
	return envVarValue, nil
}
