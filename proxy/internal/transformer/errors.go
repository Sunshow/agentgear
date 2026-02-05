package transformer

import "errors"

var (
	ErrDefinitionExists   = errors.New("transformer definition already exists")
	ErrDefinitionNotFound = errors.New("transformer definition not found")
	ErrMappingExists      = errors.New("mapping rule already exists")
	ErrMappingNotFound    = errors.New("mapping rule not found")
	ErrBuiltinReadonly    = errors.New("builtin items cannot be modified")
)
