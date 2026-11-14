package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

var (
	ErrAdtyTicketInvalid   = common.NewMessage("Invalid parameters")
	ErrAdtyTicketForbidden = common.NewMessage("No permission to perform this action")
	ErrAdtyTicketImage     = common.NewMessage("Only images are accepted")
	adtyTicketImageDir     = func() string { return filepath.Join("uploads", "adty-ticket-images") }
)

type AdtyTicketInput struct {
	Type         string `json:"type"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	Priority     string `json:"priority"`
	RelatedKind  string `json:"related_kind"`
	RelatedRef   string `json:"related_ref"`
	RelatedModel string `json:"related_model"`
	RelatedGroup string `json:"related_group"`
	RelatedError string `json:"related_error"`
	Idempotency  string `json:"-"`
}

type AdtyTicketDetail struct {
	Ticket    model.AdtyTicket          `json:"ticket"`
	Messages  []model.AdtyTicketMessage `json:"messages"`
	Images    []model.AdtyTicketImage   `json:"images"`
	Events    []model.AdtyTicketEvent   `json:"events"`
	CanNote   bool                      `json:"can_note"`
	CanManage bool                      `json:"can_manage"`
}

type AdtyTicketBadges struct {
	New               int64 `json:"new"`
	PendingAssignment int64 `json:"pending_assignment"`
	InProgress        int64 `json:"in_progress"`
	WaitingUser       int64 `json:"waiting_user"`
	WaitingInternal   int64 `json:"waiting_internal"`
	Urgent            int64 `json:"urgent"`
	Pending           int64 `json:"pending"`
}

type AdtyTicketList struct {
	Tickets   []model.AdtyTicket `json:"tickets"`
	CanNote   bool               `json:"can_note"`
	CanManage bool               `json:"can_manage"`
	Badges    AdtyTicketBadges   `json:"badges"`
}

type adtyTicketActor struct {
	User        model.User
	Staff       *model.AdtySiteStaff
	OwnerSiteId int
	TeamId      int64
	TeamRole    string
}

func SetAdtyTicketImageDirForTest(dir string) func() {
	old := adtyTicketImageDir
	adtyTicketImageDir = func() string { return dir }
	return func() { adtyTicketImageDir = old }
}

func CreateAdtyTicket(userId int, input AdtyTicketInput) (*model.AdtyTicket, error) {
	input.Title = strings.TrimSpace(redactAdtySecrets(input.Title))
	input.Body = strings.TrimSpace(redactAdtySecrets(input.Body))
	input.RelatedRef = strings.TrimSpace(input.RelatedRef)
	if userId <= 0 || input.Title == "" || input.Body == "" || len(input.Title) > 120 || adtyTextHasSecret(input.RelatedRef) {
		return nil, ErrAdtyTicketInvalid
	}
	if input.Type == "" {
		input.Type = "other"
	}
	switch input.Type {
	case "billing", "api", "account", "other":
	default:
		return nil, ErrAdtyTicketInvalid
	}
	priority := model.AdtyTicketPriorityNormal
	switch input.Priority {
	case "", model.AdtyTicketPriorityNormal, model.AdtyTicketPriorityHigh, model.AdtyTicketPriorityUrgent:
		if input.Priority != "" {
			priority = input.Priority
		}
	default:
		return nil, ErrAdtyTicketInvalid
	}
	actor, err := loadAdtyTicketActor(userId)
	if err != nil {
		return nil, err
	}
	var created *model.AdtyTicket
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		dedupe := ticketDedupe(0, input.Idempotency)
		if input.Idempotency != "" {
			dedupe = "create:" + input.Idempotency
			var prior model.AdtyTicketMessage
			lookup := tx.Where("dedupe = ?", dedupe).Limit(1).Find(&prior)
			if lookup.Error != nil {
				return lookup.Error
			}
			if lookup.RowsAffected > 0 {
				var ticket model.AdtyTicket
				if err := tx.First(&ticket, prior.TicketId).Error; err != nil {
					return err
				}
				created = &ticket
				return nil
			}
		}
		ticket := model.AdtyTicket{
			SiteId: actor.User.SiteId, UserId: actor.User.Id, TeamId: actor.TeamId, Type: input.Type,
			Title: input.Title, Priority: priority, Status: model.AdtyTicketNew,
			RelatedKind: input.RelatedKind, RelatedRef: input.RelatedRef, RelatedModel: input.RelatedModel,
			RelatedGroup: input.RelatedGroup, RelatedError: input.RelatedError,
		}
		if err := tx.Create(&ticket).Error; err != nil {
			return err
		}
		message := model.AdtyTicketMessage{
			TicketId: ticket.Id, AuthorId: actor.User.Id, Visibility: model.AdtyTicketVisibilityPublic,
			Body: input.Body, Dedupe: dedupe,
		}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.AdtyTicketEvent{TicketId: ticket.Id, ActorId: actor.User.Id, Action: "create", Detail: input.RelatedKind}).Error; err != nil {
			return err
		}
		created = &ticket
		return nil
	})
	return created, err
}

func ListAdtyTickets(userId int, queue string) (*AdtyTicketList, error) {
	actor, err := loadAdtyTicketActor(userId)
	if err != nil {
		return nil, err
	}
	scope := scopedAdtyTickets(actor)
	var rows []model.AdtyTicket
	query, err := applyAdtyTicketQueueFilter(scope.Session(&gorm.Session{}).Order("id DESC").Limit(100), queue)
	if err != nil {
		return nil, err
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Status = model.NormalizeAdtyTicketStatus(rows[i].Status, rows[i].AssigneeId)
	}
	badges, err := countAdtyTicketBadges(scope.Session(&gorm.Session{}))
	if err != nil {
		return nil, err
	}
	probe := model.AdtyTicket{SiteId: actor.scopeSite()}
	return &AdtyTicketList{Tickets: rows, CanNote: actor.canNote(probe), CanManage: actor.canManage(probe), Badges: badges}, nil
}

func scopedAdtyTickets(actor adtyTicketActor) *gorm.DB {
	query := model.DB.Model(&model.AdtyTicket{})
	switch {
	case actor.User.Role >= common.RoleRootUser:
	case actor.User.Role >= common.RoleAdminUser:
		query = query.Where("site_id = ?", 0)
	case actor.OwnerSiteId > 0:
		query = query.Where("site_id = ?", actor.OwnerSiteId)
	case actor.Staff != nil && actor.Staff.Status == "enabled":
		query = query.Where("site_id = ?", actor.Staff.SiteId)
	case actor.TeamRole == model.AdtyTeamRoleLeader && actor.TeamId > 0:
		query = query.Where("user_id = ? OR team_id = ?", actor.User.Id, actor.TeamId)
	default:
		query = query.Where("user_id = ?", actor.User.Id)
	}
	return query
}

func applyAdtyTicketQueueFilter(query *gorm.DB, queue string) (*gorm.DB, error) {
	switch strings.TrimSpace(queue) {
	case "", "all":
		return query, nil
	case "pending":
		return query.Where("status = ? OR status = ? OR (status = ? AND assignee_id = 0)",
			model.AdtyTicketNew, model.AdtyTicketPendingAssignment, model.AdtyTicketLegacyOpen), nil
	case "waiting_user":
		return query.Where("status = ?", model.AdtyTicketWaitingUser), nil
	case "in_progress":
		return query.Where("status = ? OR status = ? OR (status = ? AND assignee_id > 0)",
			model.AdtyTicketInProgress, model.AdtyTicketLegacyAssigned, model.AdtyTicketLegacyOpen), nil
	case "urgent":
		return query.Where("priority = ? AND status NOT IN ?", model.AdtyTicketPriorityUrgent, []string{model.AdtyTicketResolved, model.AdtyTicketClosed}), nil
	default:
		return nil, ErrAdtyTicketInvalid
	}
}

func countAdtyTicketBadges(scope *gorm.DB) (AdtyTicketBadges, error) {
	var badges AdtyTicketBadges
	var err error
	count := func(dest *int64, apply func(*gorm.DB) *gorm.DB) {
		if err != nil {
			return
		}
		err = apply(scope.Session(&gorm.Session{})).Count(dest).Error
	}
	count(&badges.New, func(q *gorm.DB) *gorm.DB {
		return q.Where("status = ? OR (status = ? AND assignee_id = 0)", model.AdtyTicketNew, model.AdtyTicketLegacyOpen)
	})
	count(&badges.PendingAssignment, func(q *gorm.DB) *gorm.DB {
		return q.Where("status = ?", model.AdtyTicketPendingAssignment)
	})
	count(&badges.InProgress, func(q *gorm.DB) *gorm.DB {
		return q.Where("status = ? OR status = ? OR (status = ? AND assignee_id > 0)",
			model.AdtyTicketInProgress, model.AdtyTicketLegacyAssigned, model.AdtyTicketLegacyOpen)
	})
	count(&badges.WaitingUser, func(q *gorm.DB) *gorm.DB {
		return q.Where("status = ?", model.AdtyTicketWaitingUser)
	})
	count(&badges.WaitingInternal, func(q *gorm.DB) *gorm.DB {
		return q.Where("status = ?", model.AdtyTicketWaitingInternal)
	})
	count(&badges.Urgent, func(q *gorm.DB) *gorm.DB {
		return q.Where("priority = ? AND status NOT IN ?", model.AdtyTicketPriorityUrgent, []string{model.AdtyTicketResolved, model.AdtyTicketClosed})
	})
	badges.Pending = badges.New + badges.PendingAssignment
	return badges, err
}

func GetAdtyTicket(userId int, ticketId int64) (*AdtyTicketDetail, error) {
	actor, err := loadAdtyTicketActor(userId)
	if err != nil {
		return nil, err
	}
	var ticket model.AdtyTicket
	if err := model.DB.First(&ticket, ticketId).Error; err != nil {
		return nil, err
	}
	if !actor.canRead(ticket) {
		return nil, ErrAdtyTicketForbidden
	}
	visibility := []string{model.AdtyTicketVisibilityPublic}
	if actor.canNote(ticket) {
		visibility = append(visibility, model.AdtyTicketVisibilityNote)
	}
	var messages []model.AdtyTicketMessage
	if err := model.DB.Where("ticket_id = ? AND visibility IN ?", ticket.Id, visibility).Order("id ASC").Find(&messages).Error; err != nil {
		return nil, err
	}
	var images []model.AdtyTicketImage
	if err := model.DB.Select("id", "ticket_id", "message_id", "mime", "size", "width", "height", "sha256", "created_at").Where("ticket_id = ?", ticket.Id).Order("id ASC").Find(&images).Error; err != nil {
		return nil, err
	}
	var events []model.AdtyTicketEvent
	if err := model.DB.Where("ticket_id = ?", ticket.Id).Order("id ASC").Find(&events).Error; err != nil {
		return nil, err
	}
	ticket.Status = model.NormalizeAdtyTicketStatus(ticket.Status, ticket.AssigneeId)
	return &AdtyTicketDetail{Ticket: ticket, Messages: messages, Images: images, Events: events, CanNote: actor.canNote(ticket), CanManage: actor.canManage(ticket)}, nil
}

func ReplyAdtyTicket(userId, ticketId int64, body, idempotency string) (*model.AdtyTicketMessage, error) {
	return writeAdtyTicketMessage(int(userId), ticketId, body, idempotency, model.AdtyTicketVisibilityPublic)
}

func NoteAdtyTicket(userId int, ticketId int64, body, idempotency string) (*model.AdtyTicketMessage, error) {
	return writeAdtyTicketMessage(userId, ticketId, body, idempotency, model.AdtyTicketVisibilityNote)
}

func SetAdtyTicketStatus(userId int, ticketId int64, status, priority string, assigneeId int) error {
	actor, err := loadAdtyTicketActor(userId)
	if err != nil {
		return err
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var ticket model.AdtyTicket
		if err := model.AdtyLockForUpdate(tx).First(&ticket, ticketId).Error; err != nil {
			return err
		}
		if !actor.canRead(ticket) {
			return ErrAdtyTicketForbidden
		}
		current := model.NormalizeAdtyTicketStatus(ticket.Status, ticket.AssigneeId)
		nextAssignee := ticket.AssigneeId
		if assigneeId > 0 {
			if !actor.canManage(ticket) {
				return ErrAdtyTicketForbidden
			}
			nextAssignee = assigneeId
		}
		nextPriority := ticket.Priority
		if priority != "" {
			if !actor.canManage(ticket) {
				return ErrAdtyTicketForbidden
			}
			switch priority {
			case model.AdtyTicketPriorityNormal, model.AdtyTicketPriorityHigh, model.AdtyTicketPriorityUrgent:
				nextPriority = priority
			default:
				return ErrAdtyTicketInvalid
			}
		}
		action := "status"
		next, err := nextAdtyTicketStatus(tx, actor, ticket, current, strings.TrimSpace(status), nextAssignee)
		if err != nil {
			return err
		}
		if next == "reopen_idempotent" {
			return nil
		}
		if strings.HasPrefix(next, "reopen:") {
			action = "reopen"
			next = strings.TrimPrefix(next, "reopen:")
		}
		if next == model.AdtyTicketInProgress && nextAssignee <= 0 {
			return ErrAdtyTicketInvalid
		}
		if (next == model.AdtyTicketNew || next == model.AdtyTicketPendingAssignment) && nextAssignee > 0 {
			return ErrAdtyTicketInvalid
		}
		if next == current && nextAssignee == ticket.AssigneeId && nextPriority == ticket.Priority {
			return nil
		}
		updates := map[string]any{"status": next}
		if nextPriority != ticket.Priority {
			updates["priority"] = nextPriority
		}
		if nextAssignee != ticket.AssigneeId {
			updates["assignee_id"] = nextAssignee
		}
		if err := tx.Model(&ticket).Updates(updates).Error; err != nil {
			return err
		}
		if action == "reopen" {
			return tx.Create(&model.AdtyTicketEvent{TicketId: ticket.Id, ActorId: actor.User.Id, Action: "reopen", Detail: next}).Error
		}
		if next == current && nextAssignee != ticket.AssigneeId {
			return tx.Create(&model.AdtyTicketEvent{TicketId: ticket.Id, ActorId: actor.User.Id, Action: "status", Detail: next}).Error
		}
		if next == current && nextPriority != ticket.Priority {
			return tx.Create(&model.AdtyTicketEvent{TicketId: ticket.Id, ActorId: actor.User.Id, Action: "priority", Detail: nextPriority}).Error
		}
		return tx.Create(&model.AdtyTicketEvent{TicketId: ticket.Id, ActorId: actor.User.Id, Action: action, Detail: next}).Error
	})
}

func nextAdtyTicketStatus(tx *gorm.DB, actor adtyTicketActor, ticket model.AdtyTicket, current, requested string, nextAssignee int) (string, error) {
	landing := reopenAdtyTicketLanding(nextAssignee)
	switch requested {
	case "resolve":
		if current == model.AdtyTicketClosed {
			return "", ErrAdtyTicketInvalid
		}
		return model.AdtyTicketResolved, nil
	case "reopen":
		if ticket.UserId != actor.User.Id && !actor.canManage(ticket) {
			return "", ErrAdtyTicketForbidden
		}
		if current == model.AdtyTicketResolved || current == model.AdtyTicketClosed {
			return "reopen:" + landing, nil
		}
		if idempotentAdtyTicketReopen(tx, ticket, current, ticket.AssigneeId) {
			return "reopen_idempotent", nil
		}
		return "", ErrAdtyTicketInvalid
	case "close":
		if !actor.canManage(ticket) {
			return "", ErrAdtyTicketForbidden
		}
		return model.AdtyTicketClosed, nil
	case model.AdtyTicketWaitingUser, model.AdtyTicketWaitingInternal:
		if !actor.canManage(ticket) {
			return "", ErrAdtyTicketForbidden
		}
		if !canMoveAdtyTicketToWaiting(current) {
			return "", ErrAdtyTicketInvalid
		}
		return requested, nil
	case model.AdtyTicketInProgress:
		if !actor.canManage(ticket) {
			return "", ErrAdtyTicketForbidden
		}
		if !canAssignAdtyTicket(current) {
			return "", ErrAdtyTicketInvalid
		}
		return model.AdtyTicketInProgress, nil
	case model.AdtyTicketPendingAssignment:
		if !actor.canManage(ticket) {
			return "", ErrAdtyTicketForbidden
		}
		if nextAssignee > 0 {
			return "", ErrAdtyTicketInvalid
		}
		if current != model.AdtyTicketNew && current != model.AdtyTicketPendingAssignment {
			return "", ErrAdtyTicketInvalid
		}
		return model.AdtyTicketPendingAssignment, nil
	case "":
		if nextAssignee != ticket.AssigneeId {
			if !canAssignAdtyTicket(current) {
				return "", ErrAdtyTicketInvalid
			}
			return model.AdtyTicketInProgress, nil
		}
		return current, nil
	default:
		return "", ErrAdtyTicketInvalid
	}
}

func canAssignAdtyTicket(current string) bool {
	switch current {
	case model.AdtyTicketNew, model.AdtyTicketPendingAssignment, model.AdtyTicketInProgress, model.AdtyTicketWaitingUser, model.AdtyTicketWaitingInternal:
		return true
	default:
		return false
	}
}

func canMoveAdtyTicketToWaiting(current string) bool {
	switch current {
	case model.AdtyTicketInProgress, model.AdtyTicketWaitingUser, model.AdtyTicketWaitingInternal:
		return true
	default:
		return false
	}
}

func reopenAdtyTicketLanding(assigneeId int) string {
	if assigneeId > 0 {
		return model.AdtyTicketInProgress
	}
	return model.AdtyTicketPendingAssignment
}

func idempotentAdtyTicketReopen(tx *gorm.DB, ticket model.AdtyTicket, current string, assigneeId int) bool {
	if current != reopenAdtyTicketLanding(assigneeId) {
		return false
	}
	var event model.AdtyTicketEvent
	lookup := tx.Where("ticket_id = ?", ticket.Id).Order("id DESC").Limit(1).Find(&event)
	return lookup.Error == nil && lookup.RowsAffected > 0 && event.Action == "reopen"
}

func AddAdtyTicketImage(userId int, ticketId int64, data []byte) (*model.AdtyTicketImage, error) {
	actor, err := loadAdtyTicketActor(userId)
	if err != nil {
		return nil, err
	}
	var ticket model.AdtyTicket
	if err := model.DB.First(&ticket, ticketId).Error; err != nil {
		return nil, err
	}
	if !actor.canRead(ticket) {
		return nil, ErrAdtyTicketForbidden
	}
	policy := adtyTicketPolicy()
	if len(data) == 0 || len(data) > policy.MaxBytes {
		return nil, ErrAdtyTicketImage
	}
	var count int64
	var total int64
	if err := model.DB.Model(&model.AdtyTicketImage{}).Where("ticket_id = ?", ticket.Id).Count(&count).Error; err != nil {
		return nil, err
	}
	if int(count) >= policy.MaxCount {
		return nil, ErrAdtyTicketImage
	}
	if err := model.DB.Model(&model.AdtyTicketImage{}).Where("ticket_id = ?", ticket.Id).Select("COALESCE(SUM(size),0)").Scan(&total).Error; err != nil {
		return nil, err
	}
	stripped, mime, width, height, err := sanitizeAdtyTicketImage(data, policy)
	if err != nil {
		return nil, err
	}
	if int(total)+len(stripped) > policy.MaxTotal {
		return nil, ErrAdtyTicketImage
	}
	sum := sha256.Sum256(stripped)
	dir := adtyTicketImageDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	name := strconv.FormatInt(ticket.Id, 10) + "-" + hex.EncodeToString(sum[:8]) + adtyTicketExt(mime)
	if err := os.WriteFile(filepath.Join(dir, name), stripped, 0o640); err != nil {
		return nil, err
	}
	row := model.AdtyTicketImage{TicketId: ticket.Id, Mime: mime, Size: len(stripped), Width: width, Height: height, SHA256: hex.EncodeToString(sum[:]), Storage: name}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return tx.Create(&model.AdtyTicketEvent{TicketId: ticket.Id, ActorId: actor.User.Id, Action: "image_upload", Detail: mime}).Error
	})
	return &row, err
}

func OpenAdtyTicketImage(userId int, ticketId, imageId int64) (string, string, error) {
	actor, err := loadAdtyTicketActor(userId)
	if err != nil {
		return "", "", err
	}
	var ticket model.AdtyTicket
	if err := model.DB.First(&ticket, ticketId).Error; err != nil {
		return "", "", err
	}
	if !actor.canRead(ticket) {
		return "", "", ErrAdtyTicketForbidden
	}
	var imageRow model.AdtyTicketImage
	if err := model.DB.Where("id = ? AND ticket_id = ?", imageId, ticket.Id).First(&imageRow).Error; err != nil {
		return "", "", err
	}
	if strings.Contains(imageRow.Storage, "..") || strings.ContainsAny(imageRow.Storage, `/\`) {
		return "", "", os.ErrNotExist
	}
	_ = model.DB.Create(&model.AdtyTicketEvent{TicketId: ticket.Id, ActorId: actor.User.Id, Action: "image_view", Detail: strconv.FormatInt(imageRow.Id, 10)}).Error
	return filepath.Join(adtyTicketImageDir(), imageRow.Storage), imageRow.Mime, nil
}

func writeAdtyTicketMessage(userId int, ticketId int64, body, idempotency, visibility string) (*model.AdtyTicketMessage, error) {
	body = strings.TrimSpace(redactAdtySecrets(body))
	if body == "" {
		return nil, ErrAdtyTicketInvalid
	}
	actor, err := loadAdtyTicketActor(userId)
	if err != nil {
		return nil, err
	}
	var saved *model.AdtyTicketMessage
	var notifyUser int
	var notifyID int64
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var ticket model.AdtyTicket
		if err := model.AdtyLockForUpdate(tx).First(&ticket, ticketId).Error; err != nil {
			return err
		}
		if !actor.canRead(ticket) {
			return ErrAdtyTicketForbidden
		}
		if visibility == model.AdtyTicketVisibilityNote && !actor.canNote(ticket) {
			return ErrAdtyTicketForbidden
		}
		dedupe := ticketDedupe(ticket.Id, idempotency)
		if idempotency != "" {
			var prior model.AdtyTicketMessage
			lookup := tx.Where("dedupe = ?", dedupe).Limit(1).Find(&prior)
			if lookup.Error != nil {
				return lookup.Error
			}
			if lookup.RowsAffected > 0 {
				saved = &prior
				return nil
			}
		}
		row := model.AdtyTicketMessage{TicketId: ticket.Id, AuthorId: actor.User.Id, Visibility: visibility, Body: body, Dedupe: dedupe}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		action := "reply"
		if visibility == model.AdtyTicketVisibilityNote {
			action = "note"
		}
		if err := tx.Create(&model.AdtyTicketEvent{TicketId: ticket.Id, ActorId: actor.User.Id, Action: action}).Error; err != nil {
			return err
		}
		saved = &row
		if visibility == model.AdtyTicketVisibilityPublic {
			notifyUser = ticket.UserId
			notifyID = ticket.Id
		}
		return nil
	})
	if err == nil && notifyUser > 0 && saved != nil {
		_ = QueueAdtyTelegramNotice(notifyUser, model.AdtyTelegramPurposeUser,
			"工单 "+strconv.FormatInt(notifyID, 10)+" 有新回复。请在网站查看。\n\nTicket "+strconv.FormatInt(notifyID, 10)+" has a new reply. Open the website.",
			"ticket:"+strconv.FormatInt(notifyID, 10)+":msg:"+strconv.FormatInt(saved.Id, 10))
	}
	return saved, err
}

func loadAdtyTicketActor(userId int) (adtyTicketActor, error) {
	var actor adtyTicketActor
	if err := model.DB.First(&actor.User, userId).Error; err != nil {
		return actor, err
	}
	var staff model.AdtySiteStaff
	lookup := model.DB.Where("user_id = ?", userId).Limit(1).Find(&staff)
	if lookup.Error != nil {
		return actor, lookup.Error
	}
	if lookup.RowsAffected > 0 {
		actor.Staff = &staff
	}
	var site model.AdtySite
	owned := model.DB.Where("owner_user_id = ?", userId).Limit(1).Find(&site)
	if owned.Error != nil {
		return actor, owned.Error
	}
	if owned.RowsAffected > 0 {
		actor.OwnerSiteId = site.Id
	}
	var member model.AdtyTeamMember
	team := model.DB.Where("user_id = ? AND status = ?", userId, model.AdtyTeamMemberActive).Limit(1).Find(&member)
	if team.Error != nil {
		return actor, team.Error
	}
	if team.RowsAffected > 0 {
		actor.TeamId = member.TeamId
		actor.TeamRole = member.Role
	}
	return actor, nil
}

func (actor adtyTicketActor) canRead(ticket model.AdtyTicket) bool {
	if actor.User.Role >= common.RoleRootUser {
		return true
	}
	if actor.User.Role >= common.RoleAdminUser && ticket.SiteId == 0 {
		return true
	}
	if actor.OwnerSiteId > 0 && actor.OwnerSiteId == ticket.SiteId {
		return true
	}
	if actor.Staff != nil && actor.Staff.Status == "enabled" && actor.Staff.SiteId == ticket.SiteId {
		return true
	}
	if ticket.UserId == actor.User.Id {
		return true
	}
	return actor.TeamRole == model.AdtyTeamRoleLeader && actor.TeamId > 0 && actor.TeamId == ticket.TeamId
}

func (actor adtyTicketActor) canNote(ticket model.AdtyTicket) bool {
	if ticket.UserId == actor.User.Id && actor.User.Role < common.RoleAdminUser && (actor.Staff == nil || actor.Staff.Status != "enabled") && actor.OwnerSiteId != ticket.SiteId {
		return false
	}
	return actor.canManage(ticket)
}

func (actor adtyTicketActor) canManage(ticket model.AdtyTicket) bool {
	if actor.User.Role >= common.RoleRootUser {
		return true
	}
	if actor.User.Role >= common.RoleAdminUser && ticket.SiteId == 0 {
		return true
	}
	if actor.OwnerSiteId > 0 && actor.OwnerSiteId == ticket.SiteId {
		return true
	}
	return actor.Staff != nil && actor.Staff.Status == "enabled" && actor.Staff.SiteId == ticket.SiteId
}

func (actor adtyTicketActor) scopeSite() int {
	if actor.OwnerSiteId > 0 {
		return actor.OwnerSiteId
	}
	if actor.Staff != nil {
		return actor.Staff.SiteId
	}
	return actor.User.SiteId
}

func ticketDedupe(ticketId int64, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		raw := strconv.FormatInt(time.Now().UnixNano(), 10) + ":" + strconv.FormatInt(ticketId, 10)
		sum := sha256.Sum256([]byte(raw))
		return "auto:" + hex.EncodeToString(sum[:])
	}
	return strconv.FormatInt(ticketId, 10) + ":" + key
}

func adtyTicketPolicy() model.AdtyTicketPolicy {
	policy := model.AdtyTicketPolicy{MaxBytes: 2 << 20, MaxCount: 6, MaxTotal: 8 << 20, MaxSide: 8000, MaxPixels: 16_000_000}
	var row model.AdtyTicketPolicy
	if err := model.DB.First(&row, 1).Error; err == nil {
		if row.MaxBytes > 0 {
			policy.MaxBytes = row.MaxBytes
		}
		if row.MaxCount > 0 {
			policy.MaxCount = row.MaxCount
		}
		if row.MaxTotal > 0 {
			policy.MaxTotal = row.MaxTotal
		}
		if row.MaxSide > 0 {
			policy.MaxSide = row.MaxSide
		}
		if row.MaxPixels > 0 {
			policy.MaxPixels = row.MaxPixels
		}
	}
	return policy
}

func sanitizeAdtyTicketImage(data []byte, policy model.AdtyTicketPolicy) ([]byte, string, int, int, error) {
	kind := http.DetectContentType(data)
	if bytes.HasPrefix(data, []byte("%PDF")) || strings.Contains(kind, "pdf") || strings.Contains(kind, "zip") {
		return nil, "", 0, 0, ErrAdtyTicketImage
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > policy.MaxSide || config.Height > policy.MaxSide || config.Width*config.Height > policy.MaxPixels {
		return nil, "", 0, 0, ErrAdtyTicketImage
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", 0, 0, ErrAdtyTicketImage
	}
	var buf bytes.Buffer
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		if err := jpeg.Encode(&buf, decoded, &jpeg.Options{Quality: 85}); err != nil {
			return nil, "", 0, 0, ErrAdtyTicketImage
		}
		return buf.Bytes(), "image/jpeg", config.Width, config.Height, nil
	case "png":
		if err := png.Encode(&buf, decoded); err != nil {
			return nil, "", 0, 0, ErrAdtyTicketImage
		}
		return buf.Bytes(), "image/png", config.Width, config.Height, nil
	case "gif":
		if err := gif.Encode(&buf, decoded, nil); err != nil {
			return nil, "", 0, 0, ErrAdtyTicketImage
		}
		return buf.Bytes(), "image/gif", config.Width, config.Height, nil
	default:
		if len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
			if bytes.Contains(data, []byte("EXIF")) || bytes.Contains(data, []byte("XMP ")) {
				return nil, "", 0, 0, ErrAdtyTicketImage
			}
			return data, "image/webp", config.Width, config.Height, nil
		}
		return nil, "", 0, 0, ErrAdtyTicketImage
	}
}

func adtyTicketExt(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}
