package policy

import "github.com/agentfence/agentfence/internal/config"

func MandatoryDenyPatterns() []string {
	return config.MandatoryExcludes()
}
