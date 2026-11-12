package model

const (
	AdtyTicketOpen             = "open"
	AdtyTicketAssigned         = "assigned"
	AdtyTicketWaitingUser      = "waiting_user"
	AdtyTicketWaitingInternal  = "waiting_internal"
	AdtyTicketResolved         = "resolved"
	AdtyTicketClosed           = "closed"
	AdtyTicketVisibilityPublic = "public"
	AdtyTicketVisibilityNote   = "internal"
	AdtyTicketPriorityNormal   = "normal"
	AdtyTicketPriorityHigh     = "important"
	AdtyTicketPriorityUrgent   = "urgent"
)

// AdtyTicket is one support case. Related fields are investigation hints, never a full key or upstream secret.
type AdtyTicket struct {
	Id           int64  `json:"id"`
	SiteId       int    `json:"site_id" gorm:"index"`
	UserId       int    `json:"user_id" gorm:"index"`
	TeamId       int64  `json:"team_id" gorm:"index"`
	Type         string `json:"type" gorm:"type:varchar(32)"`
	Title        string `json:"title" gorm:"type:varchar(120)"`
	Priority     string `json:"priority" gorm:"type:varchar(16);index"`
	Status       string `json:"status" gorm:"type:varchar(24);index"`
	AssigneeId   int    `json:"assignee_id" gorm:"index"`
	RelatedKind  string `json:"related_kind" gorm:"type:varchar(32)"`
	RelatedRef   string `json:"related_ref" gorm:"type:varchar(64)"`
	RelatedModel string `json:"related_model" gorm:"type:varchar(80)"`
	RelatedGroup string `json:"related_group" gorm:"type:varchar(64)"`
	RelatedError string `json:"related_error" gorm:"type:varchar(64)"`
	CreatedAt    int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AdtyTicket) TableName() string { return "adty_tickets" }

// AdtyTicketMessage is a public reply or an internal note. Customer responses omit internal rows.
type AdtyTicketMessage struct {
	Id         int64  `json:"id"`
	TicketId   int64  `json:"ticket_id" gorm:"index"`
	AuthorId   int    `json:"author_id"`
	Visibility string `json:"visibility" gorm:"type:varchar(16);index"`
	Body       string `json:"body" gorm:"type:text"`
	Dedupe     string `json:"-" gorm:"uniqueIndex;type:varchar(160)"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyTicketMessage) TableName() string { return "adty_ticket_messages" }

// AdtyTicketImage is a private raster attachment. Bytes stay on disk; this row is the authorization record.
type AdtyTicketImage struct {
	Id        int64  `json:"id"`
	TicketId  int64  `json:"ticket_id" gorm:"index"`
	MessageId int64  `json:"message_id" gorm:"index"`
	Mime      string `json:"mime" gorm:"type:varchar(32)"`
	Size      int    `json:"size"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	SHA256    string `json:"sha256" gorm:"type:varchar(64)"`
	Storage   string `json:"-" gorm:"type:varchar(128)"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyTicketImage) TableName() string { return "adty_ticket_images" }

// AdtyTicketEvent is an append-only handling record. It does not grant money or key permissions.
type AdtyTicketEvent struct {
	Id        int64  `json:"id"`
	TicketId  int64  `json:"ticket_id" gorm:"index"`
	ActorId   int    `json:"actor_id"`
	Action    string `json:"action" gorm:"type:varchar(32)"`
	Detail    string `json:"detail" gorm:"type:varchar(160)"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyTicketEvent) TableName() string { return "adty_ticket_events" }

// AdtyTicketPolicy is the Root image limit. Zero values mean the service defaults.
type AdtyTicketPolicy struct {
	Id        int   `json:"id" gorm:"primaryKey"`
	MaxBytes  int   `json:"max_bytes"`
	MaxCount  int   `json:"max_count"`
	MaxTotal  int   `json:"max_total"`
	MaxSide   int   `json:"max_side"`
	MaxPixels int   `json:"max_pixels"`
	UpdatedAt int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AdtyTicketPolicy) TableName() string { return "adty_ticket_policies" }
