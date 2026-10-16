package smartrouting

import "time"

// ScoreEntry in-memory score for channel×model.
type ScoreEntry struct {
	ChannelID    int
	Model        string
	Score        float64
	SConn        float64
	SPrice       float64
	SSpeed       float64
	SStab        float64
	SAuth        float64
	RouteWeight  int
	State        string // normal|degraded|broken|halfopen|new|floor
	Samples      int
	Insufficient bool
	FloorDecay   int // 0..FloorDecayCycles remaining
	UpdatedAt    time.Time
}

// AttemptEvent collected at relay attempt end.
type AttemptEvent struct {
	ChannelID        int
	Model            string
	UserID           int
	TokenID          int
	RequestID        string
	Success          bool
	Class            ErrClass
	LatencyMs        int64
	TTFTMs           int64
	PromptTokens     int
	CompletionTokens int
	UsageSource      string // upstream|estimate
	CacheHit         bool
	Pinned           bool
	StreamStarted    bool
	RespModel        string
	ReqModel         string
	IsStream         bool
	LocalPromptTokens int
}

// RouteDecision recorded into logs.other.route.
type RouteDecision struct {
	V         int              `json:"v"`
	Prio      int64            `json:"prio,omitempty"`
	Cands     []RouteCand      `json:"cands,omitempty"`
	Picked    int              `json:"picked,omitempty"`
	Reason    string           `json:"reason,omitempty"`
	Excluded  []RouteExcluded  `json:"excluded,omitempty"`
	Retry     int              `json:"retry,omitempty"`
	Tried     []int            `json:"tried,omitempty"`
	Pinned    bool             `json:"pinned,omitempty"`
	HalfOpen  bool             `json:"halfopen,omitempty"`
	Stage     string           `json:"stage,omitempty"`
	ScoreAt   int64            `json:"score_at,omitempty"`
	Degraded  bool             `json:"degraded,omitempty"`
}

type RouteCand struct {
	ID    int     `json:"id"`
	W     int     `json:"w"`
	Score float64 `json:"score,omitempty"`
	RW    int     `json:"rw,omitempty"`
	Note  string  `json:"note,omitempty"`
}

type RouteExcluded struct {
	ID  int    `json:"id"`
	Why string `json:"why"`
}

// MinuteBucket aggregates one Redis TIME minute.
type MinuteBucket struct {
	OK           float64
	Fail         float64
	LatencySumMs float64
	LatencyN     int
	TokenSamples map[int]float64 // tokenID -> weighted samples
	UserFail     map[int]float64
	UserTotal    map[int]float64
}
