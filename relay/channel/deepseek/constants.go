package deepseek

var ModelList = []string{
	"deepseek-flash",
	"deepseek-v4-pro",
}

var ChannelName = "deepseek"

// Tool-price keys of the DeepSeek Search and Fetch APIs. They price the
// server-side searches and page fetches DeepSeek runs on its Anthropic and
// Responses endpoints.
const (
	SearchToolPriceKey = "deepseek_search"
	FetchToolPriceKey  = "deepseek_fetch"
)
