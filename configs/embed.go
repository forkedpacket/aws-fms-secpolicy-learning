package configs

import _ "embed"

// EmbeddedPolicyVariants exposes the default policy-variants.yaml for Lambda/CLI to load without filesystem access.
//
//go:embed policy-variants.yaml
var EmbeddedPolicyVariants []byte
