// Package config provides a generic, multi-source configuration loader.
//
// Sources are applied in ascending precedence order so that higher-priority
// sources win:
//
//  1. Struct defaults   (lowest — applied first)
//  2. YAML file
//  3. .env file
//  4. Environment variables
//  5. Explicit overrides (highest)
//
// Schema is driven by struct tags:
//
//	type ServerConfig struct {
//	    Host    string `yaml:"host"    env:"HOST"    default:"localhost"`
//	    Port    int    `yaml:"port"    env:"PORT"    default:"8080"     validate:"required"`
//	    Secret  string `yaml:"secret"  env:"SECRET"                    validate:"required"`
//	}
//
// After loading, if the struct implements [Validator], Validate is called
// automatically so domain-specific constraints live in one place.
package config
