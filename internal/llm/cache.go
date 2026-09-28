package llm

type CacheControl struct {
	Type string
	TTL  string
}

func CacheEphemeral() *CacheControl {
	return &CacheControl{Type: "ephemeral"}
}

func CacheEphemeral1h() *CacheControl {
	return &CacheControl{Type: "ephemeral", TTL: "1h"}
}
