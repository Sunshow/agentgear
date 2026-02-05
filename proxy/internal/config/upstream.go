package config

import "strings"

const (
	UpstreamTypeWarp      = "warp"
	UpstreamTypeKiro      = "kiro"
	UpstreamTypeAnthropic = "anthropic"
	UpstreamTypeOpenAI    = "openai"
)

const UpstreamTagPrefix = "$u_"
const GatewayTagPrefix = "$g_"

func GetUpstreamTag(upstreamType string) string {
	if upstreamType == "" {
		return ""
	}
	return UpstreamTagPrefix + strings.ToLower(upstreamType)
}

func GetGatewayTag(gatewayName string) string {
	if gatewayName == "" {
		return ""
	}
	return GatewayTagPrefix + strings.ToLower(gatewayName)
}
