package config

import (
	"bufio"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Validator is an optional interface that a config struct can implement.
// Load calls Validate after all sources have been merged, returning any error.
type Validator interface {
	Validate() error
}

type options struct {
	yamlFile  string
	envFile   string
	overrides map[string]string
}

// Option configures the behaviour of Load.
type Option func(*options)

// WithYAMLFile tells the loader to read configuration from the given YAML file.
// The file must exist; a missing file is an error.
func WithYAMLFile(path string) Option {
	return func(o *options) { o.yamlFile = path }
}

// WithEnvFile tells the loader to read KEY=VALUE pairs from the given file.
// Lines starting with # and blank lines are ignored.
// The file must exist; a missing file is an error.
func WithEnvFile(path string) Option {
	return func(o *options) { o.envFile = path }
}

// WithOverrides injects caller-supplied string values at the highest precedence.
// Keys must match the env: struct tag of the target field (e.g. "PORT").
func WithOverrides(m map[string]string) Option {
	return func(o *options) { o.overrides = m }
}

// Load constructs a value of type T by merging all configured sources in
// precedence order, then calls Validate if T implements Validator.
func Load[T any](opts ...Option) (T, error) {
	var cfg T
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	rv := reflect.ValueOf(&cfg).Elem()

	if err := applyDefaults(rv); err != nil {
		return cfg, fmt.Errorf("config: defaults: %w", err)
	}
	if o.yamlFile != "" {
		if err := applyYAMLFile(o.yamlFile, &cfg); err != nil {
			return cfg, fmt.Errorf("config: yaml: %w", err)
		}
	}
	envFileVars := map[string]string{}
	if o.envFile != "" {
		var err error
		envFileVars, err = parseEnvFile(o.envFile)
		if err != nil {
			return cfg, fmt.Errorf("config: env file: %w", err)
		}
	}
	if err := applyEnvVars(rv, envFileVars); err != nil {
		return cfg, fmt.Errorf("config: env vars: %w", err)
	}
	if len(o.overrides) > 0 {
		// Overrides do not fall back to os.Getenv — explicit map wins unconditionally.
		if err := applyMapOnly(rv, o.overrides); err != nil {
			return cfg, fmt.Errorf("config: overrides: %w", err)
		}
	}
	if err := checkRequired(rv); err != nil {
		return cfg, err
	}
	if v, ok := any(&cfg).(Validator); ok {
		if err := v.Validate(); err != nil {
			return cfg, fmt.Errorf("config: validate: %w", err)
		}
	}
	return cfg, nil
}

func applyDefaults(rv reflect.Value) error {
	rt := rv.Type()
	for i := range rt.NumField() {
		field := rt.Field(i)
		fv := rv.Field(i)
		if !fv.CanSet() {
			continue
		}
		if field.Type.Kind() == reflect.Struct && field.Type != reflect.TypeOf(time.Time{}) {
			if err := applyDefaults(fv); err != nil {
				return err
			}
			continue
		}
		def, ok := field.Tag.Lookup("default")
		if !ok {
			continue
		}
		if !fv.IsZero() {
			continue
		}
		if err := setField(fv, field.Name, def); err != nil {
			return err
		}
	}
	return nil
}

func applyYAMLFile(path string, cfg any) error {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close() //nolint:errcheck

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		return fmt.Errorf("decode %q: %w", path, err)
	}
	return nil
}

func parseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close() //nolint:errcheck

	out := map[string]string{}
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("line %d: missing '=' in %q", lineNum, line)
		}
		out[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// applyEnvVars applies values from envFileVars, then overrides with os.Getenv.
func applyEnvVars(rv reflect.Value, envFileVars map[string]string) error {
	rt := rv.Type()
	for i := range rt.NumField() {
		field := rt.Field(i)
		fv := rv.Field(i)
		if !fv.CanSet() {
			continue
		}
		if field.Type.Kind() == reflect.Struct && field.Type != reflect.TypeOf(time.Time{}) {
			if err := applyEnvVars(fv, envFileVars); err != nil {
				return err
			}
			continue
		}
		envKey, ok := field.Tag.Lookup("env")
		if !ok || envKey == "" {
			continue
		}
		val := ""
		if v, found := envFileVars[envKey]; found {
			val = v
		}
		// System env var wins over .env file.
		if sysVal := os.Getenv(envKey); sysVal != "" {
			val = sysVal
		}
		if val == "" {
			continue
		}
		if err := setField(fv, field.Name, val); err != nil {
			return err
		}
	}
	return nil
}

// applyMapOnly sets fields from m by their env: tag without checking os.Getenv.
// Used for explicit overrides that must win over system env vars.
func applyMapOnly(rv reflect.Value, m map[string]string) error {
	rt := rv.Type()
	for i := range rt.NumField() {
		field := rt.Field(i)
		fv := rv.Field(i)
		if !fv.CanSet() {
			continue
		}
		if field.Type.Kind() == reflect.Struct && field.Type != reflect.TypeOf(time.Time{}) {
			if err := applyMapOnly(fv, m); err != nil {
				return err
			}
			continue
		}
		envKey, ok := field.Tag.Lookup("env")
		if !ok || envKey == "" {
			continue
		}
		val, found := m[envKey]
		if !found || val == "" {
			continue
		}
		if err := setField(fv, field.Name, val); err != nil {
			return err
		}
	}
	return nil
}

func checkRequired(rv reflect.Value) error {
	rt := rv.Type()
	for i := range rt.NumField() {
		field := rt.Field(i)
		fv := rv.Field(i)
		if field.Type.Kind() == reflect.Struct && field.Type != reflect.TypeOf(time.Time{}) {
			if err := checkRequired(fv); err != nil {
				return err
			}
			continue
		}
		tag := field.Tag.Get("validate")
		if !strings.Contains(tag, "required") {
			continue
		}
		if fv.IsZero() {
			envKey := field.Tag.Get("env")
			yamlKey := field.Tag.Get("yaml")
			hint := field.Name
			if envKey != "" {
				hint = envKey
			} else if yamlKey != "" {
				hint = yamlKey
			}
			return fmt.Errorf("config: field %q is required but not set", hint)
		}
	}
	return nil
}

func setField(fv reflect.Value, fieldName, s string) error {
	switch fv.Kind() { //nolint:exhaustive
	case reflect.String:
		fv.SetString(s)
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return fmt.Errorf("field %q: cannot parse %q as bool: %w", fieldName, s, err)
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if fv.Type() == reflect.TypeOf(time.Duration(0)) {
			d, err := time.ParseDuration(s)
			if err != nil {
				return fmt.Errorf("field %q: cannot parse %q as duration: %w", fieldName, s, err)
			}
			fv.SetInt(int64(d))
			return nil
		}
		n, err := strconv.ParseInt(s, 10, fv.Type().Bits())
		if err != nil {
			return fmt.Errorf("field %q: cannot parse %q as int: %w", fieldName, s, err)
		}
		fv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(s, 10, fv.Type().Bits())
		if err != nil {
			return fmt.Errorf("field %q: cannot parse %q as uint: %w", fieldName, s, err)
		}
		fv.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(s, fv.Type().Bits())
		if err != nil {
			return fmt.Errorf("field %q: cannot parse %q as float: %w", fieldName, s, err)
		}
		fv.SetFloat(f)
	default:
		return fmt.Errorf("field %q: unsupported type %s", fieldName, fv.Type())
	}
	return nil
}
