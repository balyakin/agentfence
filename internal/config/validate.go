package config

import (
	"fmt"
	"regexp"

	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/go-playground/validator/v10"
	"github.com/oklog/ulid/v2"
)

var envKeyPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

func NewValidator() (*validator.Validate, error) {
	validate := validator.New()
	if err := validate.RegisterValidation("envkey", func(fl validator.FieldLevel) bool {
		return envKeyPattern.MatchString(fl.Field().String())
	}); err != nil {
		return nil, errorsx.Wrap(errorsx.CodeInternal, "validator bootstrap failed", errorsx.ExitInternal, err)
	}
	if err := validate.RegisterValidation("ulid", func(fl validator.FieldLevel) bool {
		_, err := ulid.Parse(fl.Field().String())
		return err == nil
	}); err != nil {
		return nil, errorsx.Wrap(errorsx.CodeInternal, "validator bootstrap failed", errorsx.ExitInternal, err)
	}
	return validate, nil
}

func Validate(cfg Config) error {
	validate, err := NewValidator()
	if err != nil {
		return err
	}
	if err := validate.Struct(cfg); err != nil {
		return errorsx.Wrap(errorsx.CodeConfigInvalid, "config invalid", errorsx.ExitUsage, err)
	}
	if _, ok := cfg.Agent.Adapters[cfg.Agent.Default]; !ok {
		return errorsx.Wrap(errorsx.CodeConfigInvalid, fmt.Sprintf("agent.default %q has no adapter", cfg.Agent.Default), errorsx.ExitUsage, nil)
	}
	return nil
}
