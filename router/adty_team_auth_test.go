package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const teamAuthMainHost = "localhost"

type teamAuthFixture struct {
	db                      *gorm.DB
	engine                  *gin.Engine
	siteA, siteB            *model.AdtySite
	root, ownerA, ownerB    *model.User
	mainUser, mainLeader    *model.User
	leaderA, memberA, userA *model.User
	leaderB, staffA         *model.User
	rootTok, ownerATok      string
	ownerBTok, mainUserTok  string
	mainLeaderTok           string
	leaderATok, memberATok  string
	userATok, leaderBTok    string
	staffATok               string
}

type teamAuthCreated struct {
	Team   model.AdtyTeam
	Member model.AdtyTeamMember
}

func newTeamAuthFixture(t *testing.T) *teamAuthFixture {
	t.Helper()
	disableAdtyRedis(t)
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Token{}, &model.AuthFlow{}, &model.TwoFA{}, &model.PasskeyCredential{},
		&model.AuditLog{}, &model.Log{}, &model.Option{}, &model.TopUp{}, &model.UserAccessToken{}, &model.AdtyUserFingerprint{}))
	require.NoError(t, model.MigrateAdtyTables())
	cfg := withResellerSetting(t)
	cfg.Enabled, cfg.RootDomains = true, []string{"sites.example.com"}
	cfg.CostFactor, cfg.MinCostFactor, cfg.MaxMarkup = new(0.8), new(0.5), new(3.0)
	cfg.MaxSiteBonusExtra, cfg.TrialGrantMaxUSD, cfg.TrialGrantDailyUSD = new(2.0), new(5.0), new(20.0)
	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)

	f := &teamAuthFixture{db: db}
	f.root = f.createUser(t, "team-auth-root", common.RoleRootUser, 0, 0)
	f.mainUser = f.createUser(t, "team-auth-main-user", common.RoleCommonUser, 0, 0)
	f.mainLeader = f.createUser(t, "team-auth-main-leader", common.RoleCommonUser, 0, 0)
	f.ownerA = f.createUser(t, "team-auth-owner-a", common.RoleCommonUser, 0, 0)
	f.ownerB = f.createUser(t, "team-auth-owner-b", common.RoleCommonUser, 0, 0)

	siteA, err := service.OpenAdtySite(f.ownerA.Id, "team-auth-a", model.AdtySiteOpenedByAdmin, service.AdtySitePreset{})
	require.NoError(t, err)
	siteB, err := service.OpenAdtySite(f.ownerB.Id, "team-auth-b", model.AdtySiteOpenedByAdmin, service.AdtySitePreset{})
	require.NoError(t, err)
	current := service.AdtyCurrentSiteAgreementVersion()
	require.NoError(t, db.Model(&model.AdtySite{}).Where("id IN ?", []int{siteA.Id, siteB.Id}).Update("agreement_ver", current).Error)
	require.NoError(t, db.First(siteA, siteA.Id).Error)
	require.NoError(t, db.First(siteB, siteB.Id).Error)
	siteA.AgreementVer, siteB.AgreementVer = current, current
	f.siteA, f.siteB = siteA, siteB
	require.Equal(t, 0, f.ownerA.SiteId)

	f.leaderA = f.createUser(t, "team-auth-leader-a", common.RoleCommonUser, siteA.Id, 0)
	f.memberA = f.createUser(t, "team-auth-member-a", common.RoleCommonUser, siteA.Id, 0)
	f.userA = f.createUser(t, "team-auth-user-a", common.RoleCommonUser, siteA.Id, 0)
	f.leaderB = f.createUser(t, "team-auth-leader-b", common.RoleCommonUser, siteB.Id, 0)
	f.staffA = f.createUser(t, "team-auth-staff-a", common.RoleCommonUser, siteA.Id, siteA.Id)

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		if requestSite := service.AdtySiteForHost(c.Request.Host); requestSite != nil {
			c.Set(service.AdtySiteContextKey, requestSite)
		}
		c.Next()
	})
	SetApiRouter(engine)
	f.engine = engine

	f.rootTok = f.session(t, f.root)
	f.ownerATok = f.session(t, f.ownerA)
	f.ownerBTok = f.session(t, f.ownerB)
	f.mainUserTok = f.session(t, f.mainUser)
	f.mainLeaderTok = f.session(t, f.mainLeader)
	f.leaderATok = f.session(t, f.leaderA)
	f.memberATok = f.session(t, f.memberA)
	f.userATok = f.session(t, f.userA)
	f.leaderBTok = f.session(t, f.leaderB)
	f.staffATok = f.session(t, f.staffA)
	return f
}

func (f *teamAuthFixture) createUser(t *testing.T, username string, role, siteID, staffSiteID int) *model.User {
	t.Helper()
	user := &model.User{
		Username: username, Password: "hashed-password", Role: role, Status: common.UserStatusEnabled,
		Group: "default", AuthVersion: 1, SiteId: siteID, StaffSiteId: staffSiteID, Email: username + "@example.com",
	}
	require.NoError(t, f.db.Create(user).Error)
	return user
}

func (f *teamAuthFixture) session(t *testing.T, user *model.User) string {
	t.Helper()
	bundle, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "team-auth-test")
	require.NoError(t, err)
	require.NotEmpty(t, bundle.AccessToken)
	return bundle.AccessToken
}

func (f *teamAuthFixture) siteAHost() string { return service.AdtySiteDisplayHost(f.siteA) }
func (f *teamAuthFixture) siteBHost() string { return service.AdtySiteDisplayHost(f.siteB) }

func (f *teamAuthFixture) call(t *testing.T, method, host, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	return adtyJSONOnHost(t, f.engine, host, method, path, body, token)
}

func (f *teamAuthFixture) teamCount(t *testing.T) int64 {
	t.Helper()
	var n int64
	require.NoError(t, f.db.Model(&model.AdtyTeam{}).Count(&n).Error)
	return n
}

func (f *teamAuthFixture) decodeCreated(t *testing.T, rec *httptest.ResponseRecorder) teamAuthCreated {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decodeAdtyData[teamAuthCreated](t, rec.Body)
}

func teamAuthRejected(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	var envelope struct {
		Success bool `json:"success"`
	}
	if rec.Body.Len() > 0 && json.Valid(rec.Body.Bytes()) {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
		assert.False(t, envelope.Success, rec.Body.String())
		return
	}
	assert.NotEqual(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestOrdinaryUserCannotSelfCreateTeamOrDiscount(t *testing.T) {
	f := newTeamAuthFixture(t)
	start := f.teamCount(t)
	body := `{"name":"Self Serve","discount":"0.50"}`
	for _, tc := range []struct{ name, host, token string }{
		{"main user on main host", teamAuthMainHost, f.mainUserTok},
		{"site customer on own host", f.siteAHost(), f.userATok},
		{"site leader on own host", f.siteAHost(), f.leaderATok},
		{"staff on own host", f.siteAHost(), f.staffATok},
		{"owner on own host via old path", f.siteAHost(), f.ownerATok},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := f.call(t, http.MethodPost, tc.host, "/api/adty/teams", body, tc.token)
			teamAuthRejected(t, rec)
		})
	}
	assert.Equal(t, start, f.teamCount(t))
}

func TestRootCreatesMainSiteTeamWithDesignatedLeader(t *testing.T) {
	f := newTeamAuthFixture(t)
	rec := f.call(t, http.MethodPost, teamAuthMainHost, "/api/adty/admin/teams",
		fmt.Sprintf(`{"name":"Main Desk","leader_id":%d,"discount":"0.80"}`, f.mainLeader.Id), f.rootTok)
	created := f.decodeCreated(t, rec)
	assert.Equal(t, 0, created.Team.SiteId)
	assert.Equal(t, f.mainLeader.Id, created.Team.LeaderUserId)
	assert.NotEqual(t, f.root.Id, created.Team.LeaderUserId)
	assert.True(t, created.Team.Discount.Equal(decimal.RequireFromString("0.80")))
	assert.Equal(t, model.AdtyTeamRoleLeader, created.Member.Role)
	assert.Equal(t, f.mainLeader.Id, created.Member.UserId)

	defaulted := f.decodeCreated(t, f.call(t, http.MethodPost, teamAuthMainHost, "/api/adty/admin/teams",
		fmt.Sprintf(`{"name":"Default Price","leader_id":%d}`, f.mainUser.Id), f.rootTok))
	assert.Equal(t, 0, defaulted.Team.SiteId)
	assert.Equal(t, f.mainUser.Id, defaulted.Team.LeaderUserId)
	assert.True(t, defaulted.Team.Discount.Equal(decimal.NewFromInt(1)))

	denied := f.call(t, http.MethodPost, teamAuthMainHost, "/api/adty/admin/teams",
		fmt.Sprintf(`{"name":"Not Root","leader_id":%d,"discount":"0.80"}`, f.mainLeader.Id), f.mainUserTok)
	teamAuthRejected(t, denied)
}

func TestOwnerCreatesTeamOnOwnSiteOnly(t *testing.T) {
	f := newTeamAuthFixture(t)
	start := f.teamCount(t)

	rec := f.call(t, http.MethodPost, f.siteAHost(), "/api/adty/site/teams",
		fmt.Sprintf(`{"name":"Site Desk","leader_id":%d,"discount":"0.90"}`, f.leaderA.Id), f.ownerATok)
	created := f.decodeCreated(t, rec)
	assert.Equal(t, f.siteA.Id, created.Team.SiteId)
	assert.NotEqual(t, 0, created.Team.SiteId)
	assert.Equal(t, f.leaderA.Id, created.Team.LeaderUserId)
	assert.NotEqual(t, f.ownerA.Id, created.Team.LeaderUserId)
	assert.True(t, created.Team.Discount.Equal(decimal.RequireFromString("0.90")))

	// Owner cannot use the Root main-site create path.
	teamAuthRejected(t, f.call(t, http.MethodPost, teamAuthMainHost, "/api/adty/admin/teams",
		fmt.Sprintf(`{"name":"Main From Owner","leader_id":%d,"discount":"1"}`, f.mainLeader.Id), f.ownerATok))

	// Other site's owner, staff, and ordinary users cannot create on this site.
	for _, tc := range []struct{ name, host, token string }{
		{"other owner on site A host", f.siteAHost(), f.ownerBTok},
		{"other owner creating via own site does not touch site A", f.siteBHost(), f.ownerBTok},
		{"staff on site A", f.siteAHost(), f.staffATok},
		{"customer on site A", f.siteAHost(), f.userATok},
		{"main user on site A", f.siteAHost(), f.mainUserTok},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := f.teamCount(t)
			body := fmt.Sprintf(`{"name":"Denied","leader_id":%d,"discount":"0.90"}`, f.leaderA.Id)
			// Owner B on site B would designate a site-A leader, which must fail.
			if tc.token == f.ownerBTok && tc.host == f.siteBHost() {
				body = fmt.Sprintf(`{"name":"Denied","leader_id":%d,"discount":"0.90"}`, f.leaderA.Id)
			}
			teamAuthRejected(t, f.call(t, http.MethodPost, tc.host, "/api/adty/site/teams", body, tc.token))
			if tc.token == f.ownerBTok && tc.host == f.siteBHost() {
				assert.Equal(t, before, f.teamCount(t), "owner B must not create a team with a site A leader")
			}
		})
	}
	assert.Equal(t, start+1, f.teamCount(t))
}

func TestDesignatedLeaderMustBeEligibleSameSiteUser(t *testing.T) {
	f := newTeamAuthFixture(t)
	start := f.teamCount(t)
	ok := f.decodeCreated(t, f.call(t, http.MethodPost, f.siteAHost(), "/api/adty/site/teams",
		fmt.Sprintf(`{"name":"Site Desk","leader_id":%d,"discount":"0.90"}`, f.leaderA.Id), f.ownerATok))
	require.Equal(t, f.leaderA.Id, ok.Team.LeaderUserId)

	for _, tc := range []struct {
		name     string
		leaderID int
	}{
		{"missing user", 9_000_001},
		{"other site user", f.leaderB.Id},
		{"staff", f.staffA.Id},
		{"already leads a team", f.leaderA.Id},
		{"main-site user", f.mainLeader.Id},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"name":"Bad Leader","leader_id":%d,"discount":"0.90"}`, tc.leaderID)
			rec := f.call(t, http.MethodPost, f.siteAHost(), "/api/adty/site/teams", body, f.ownerATok)
			teamAuthRejected(t, rec)
			if tc.name == "missing user" || tc.name == "other site user" {
				assert.NotContains(t, rec.Body.String(), "record not found")
			}
		})
	}

	// A current member also cannot be designated as a new team's leader.
	invite := f.call(t, http.MethodPost, f.siteAHost(), fmt.Sprintf("/api/adty/teams/%d/invites", ok.Team.Id), "", f.leaderATok)
	require.Equal(t, http.StatusOK, invite.Code, invite.Body.String())
	code := decodeAdtyData[struct {
		Code string `json:"code"`
	}](t, invite.Body).Code
	accepted := f.call(t, http.MethodPost, f.siteAHost(), "/api/adty/teams/accept", fmt.Sprintf(`{"code":%q}`, code), f.memberATok)
	require.Equal(t, http.StatusOK, accepted.Code, accepted.Body.String())
	teamAuthRejected(t, f.call(t, http.MethodPost, f.siteAHost(), "/api/adty/site/teams",
		fmt.Sprintf(`{"name":"Already Member","leader_id":%d,"discount":"0.90"}`, f.memberA.Id), f.ownerATok))
	assert.Equal(t, start+1, f.teamCount(t))
}

func TestOnlyRootOrAuthorizedOwnerCanSetProtocolDiscount(t *testing.T) {
	f := newTeamAuthFixture(t)
	mainTeam := f.decodeCreated(t, f.call(t, http.MethodPost, teamAuthMainHost, "/api/adty/admin/teams",
		fmt.Sprintf(`{"name":"Main Desk","leader_id":%d,"discount":"1"}`, f.mainLeader.Id), f.rootTok))
	siteTeam := f.decodeCreated(t, f.call(t, http.MethodPost, f.siteAHost(), "/api/adty/site/teams",
		fmt.Sprintf(`{"name":"Site Desk","leader_id":%d,"discount":"0.90"}`, f.leaderA.Id), f.ownerATok))

	okMain := f.call(t, http.MethodPut, teamAuthMainHost, fmt.Sprintf("/api/adty/admin/teams/%d/agreement-price", mainTeam.Team.Id),
		`{"discount":"0.70"}`, f.rootTok)
	require.Equal(t, http.StatusOK, okMain.Code, okMain.Body.String())
	okSite := f.call(t, http.MethodPut, f.siteAHost(), fmt.Sprintf("/api/adty/site/teams/%d/discount", siteTeam.Team.Id),
		`{"discount":"0.85"}`, f.ownerATok)
	require.Equal(t, http.StatusOK, okSite.Code, okSite.Body.String())

	teamAuthRejected(t, f.call(t, http.MethodPut, teamAuthMainHost, fmt.Sprintf("/api/adty/admin/teams/%d/agreement-price", mainTeam.Team.Id),
		`{"discount":"0.60"}`, f.mainUserTok))
	teamAuthRejected(t, f.call(t, http.MethodPut, teamAuthMainHost, fmt.Sprintf("/api/adty/admin/teams/%d/agreement-price", mainTeam.Team.Id),
		`{"discount":"0.60"}`, f.mainLeaderTok))
	teamAuthRejected(t, f.call(t, http.MethodPut, f.siteAHost(), fmt.Sprintf("/api/adty/site/teams/%d/discount", siteTeam.Team.Id),
		`{"discount":"0.95"}`, f.userATok))
	teamAuthRejected(t, f.call(t, http.MethodPut, f.siteAHost(), fmt.Sprintf("/api/adty/site/teams/%d/discount", siteTeam.Team.Id),
		`{"discount":"0.95"}`, f.leaderATok))
	teamAuthRejected(t, f.call(t, http.MethodPut, f.siteAHost(), fmt.Sprintf("/api/adty/site/teams/%d/discount", siteTeam.Team.Id),
		`{"discount":"0.95"}`, f.staffATok))
	teamAuthRejected(t, f.call(t, http.MethodPut, f.siteAHost(), fmt.Sprintf("/api/adty/site/teams/%d/discount", siteTeam.Team.Id),
		`{"discount":"0.95"}`, f.ownerBTok))
	// Root agreement-price is main-site only.
	teamAuthRejected(t, f.call(t, http.MethodPut, teamAuthMainHost, fmt.Sprintf("/api/adty/admin/teams/%d/agreement-price", siteTeam.Team.Id),
		`{"discount":"0.70"}`, f.rootTok))

	var storedMain, storedSite model.AdtyTeam
	require.NoError(t, f.db.First(&storedMain, mainTeam.Team.Id).Error)
	require.NoError(t, f.db.First(&storedSite, siteTeam.Team.Id).Error)
	assert.True(t, storedMain.Discount.Equal(decimal.RequireFromString("0.70")))
	assert.True(t, storedSite.Discount.Equal(decimal.RequireFromString("0.85")))
}

func TestAuthorizedTeamInviteAcceptAndMineStillWork(t *testing.T) {
	f := newTeamAuthFixture(t)
	created := f.decodeCreated(t, f.call(t, http.MethodPost, f.siteAHost(), "/api/adty/site/teams",
		fmt.Sprintf(`{"name":"Site Desk","leader_id":%d,"discount":"0.90"}`, f.leaderA.Id), f.ownerATok))

	mine := f.call(t, http.MethodGet, f.siteAHost(), "/api/adty/teams/mine", "", f.leaderATok)
	require.Equal(t, http.StatusOK, mine.Code, mine.Body.String())
	assert.Contains(t, mine.Body.String(), "Site Desk")
	assert.Contains(t, mine.Body.String(), `"role":"leader"`)

	invite := f.call(t, http.MethodPost, f.siteAHost(), fmt.Sprintf("/api/adty/teams/%d/invites", created.Team.Id), "", f.leaderATok)
	require.Equal(t, http.StatusOK, invite.Code, invite.Body.String())
	code := decodeAdtyData[struct {
		Code string `json:"code"`
	}](t, invite.Body).Code
	require.NotEmpty(t, code)

	accepted := f.call(t, http.MethodPost, f.siteAHost(), "/api/adty/teams/accept", fmt.Sprintf(`{"code":%q}`, code), f.memberATok)
	require.Equal(t, http.StatusOK, accepted.Code, accepted.Body.String())

	memberMine := f.call(t, http.MethodGet, f.siteAHost(), "/api/adty/teams/mine", "", f.memberATok)
	require.Equal(t, http.StatusOK, memberMine.Code, memberMine.Body.String())
	assert.Contains(t, memberMine.Body.String(), "Site Desk")
	assert.Contains(t, memberMine.Body.String(), `"role":"member"`)
}

func TestAuthorizedTeamInviteTTLAndRevoke(t *testing.T) {
	f := newTeamAuthFixture(t)
	created := f.decodeCreated(t, f.call(t, http.MethodPost, f.siteAHost(), "/api/adty/site/teams",
		fmt.Sprintf(`{"name":"Invite Desk","leader_id":%d,"discount":"0.90"}`, f.leaderA.Id), f.ownerATok))

	now := time.Now().Unix()
	defaultInvite := f.call(t, http.MethodPost, f.siteAHost(), fmt.Sprintf("/api/adty/teams/%d/invites", created.Team.Id), "", f.leaderATok)
	require.Equal(t, http.StatusOK, defaultInvite.Code, defaultInvite.Body.String())
	defaultIssued := decodeAdtyData[service.AdtyTeamInviteIssue](t, defaultInvite.Body)
	require.NotEmpty(t, defaultIssued.Code)
	assert.InDelta(t, now+int64(service.AdtyTeamInviteDefaultTTL.Seconds()), defaultIssued.ExpiresAt, 5)
	assert.NotContains(t, defaultInvite.Body.String(), "code_hash")

	customInvite := f.call(t, http.MethodPost, f.siteAHost(), fmt.Sprintf("/api/adty/teams/%d/invites", created.Team.Id), `{"ttl_seconds":3600}`, f.leaderATok)
	require.Equal(t, http.StatusOK, customInvite.Code, customInvite.Body.String())
	customIssued := decodeAdtyData[service.AdtyTeamInviteIssue](t, customInvite.Body)
	assert.InDelta(t, now+3600, customIssued.ExpiresAt, 5)
	assert.Less(t, customIssued.ExpiresAt, defaultIssued.ExpiresAt)

	teamAuthRejected(t, f.call(t, http.MethodPost, f.siteAHost(), fmt.Sprintf("/api/adty/teams/%d/invites", created.Team.Id), `{"ttl_seconds":0}`, f.leaderATok))
	teamAuthRejected(t, f.call(t, http.MethodPost, f.siteAHost(), fmt.Sprintf("/api/adty/teams/%d/invites", created.Team.Id), `{"ttl_seconds":-1}`, f.leaderATok))

	toRevoke := f.call(t, http.MethodPost, f.siteAHost(), fmt.Sprintf("/api/adty/teams/%d/invites", created.Team.Id), `{"ttl_seconds":7200}`, f.leaderATok)
	require.Equal(t, http.StatusOK, toRevoke.Code, toRevoke.Body.String())
	revocable := decodeAdtyData[service.AdtyTeamInviteIssue](t, toRevoke.Body)

	for _, tc := range []struct{ name, token string }{
		{"member", f.memberATok},
		{"ordinary user", f.userATok},
		{"staff", f.staffATok},
		{"other site leader", f.leaderBTok},
		{"site owner", f.ownerATok},
		{"root", f.rootTok},
	} {
		t.Run("unauthorized revoke "+tc.name, func(t *testing.T) {
			teamAuthRejected(t, f.call(t, http.MethodPost, f.siteAHost(),
				fmt.Sprintf("/api/adty/teams/%d/invites/%d/revoke", created.Team.Id, revocable.Id), "", tc.token))
		})
		t.Run("unauthorized list "+tc.name, func(t *testing.T) {
			teamAuthRejected(t, f.call(t, http.MethodGet, f.siteAHost(),
				fmt.Sprintf("/api/adty/teams/%d/invites", created.Team.Id), "", tc.token))
		})
	}

	revoked := f.call(t, http.MethodPost, f.siteAHost(),
		fmt.Sprintf("/api/adty/teams/%d/invites/%d/revoke", created.Team.Id, revocable.Id), "", f.leaderATok)
	require.Equal(t, http.StatusOK, revoked.Code, revoked.Body.String())
	view := decodeAdtyData[service.AdtyTeamInviteView](t, revoked.Body)
	assert.Equal(t, model.AdtyTeamInviteStatusRevoked, view.Status)
	assert.Equal(t, f.leaderA.Id, view.CreatedBy)
	assert.Equal(t, f.leaderA.Id, view.RevokedBy)
	teamAuthRejected(t, f.call(t, http.MethodPost, f.siteAHost(), "/api/adty/teams/accept",
		fmt.Sprintf(`{"code":%q}`, revocable.Code), f.memberATok))

	usedInvite := f.call(t, http.MethodPost, f.siteAHost(), fmt.Sprintf("/api/adty/teams/%d/invites", created.Team.Id), "", f.leaderATok)
	require.Equal(t, http.StatusOK, usedInvite.Code, usedInvite.Body.String())
	used := decodeAdtyData[service.AdtyTeamInviteIssue](t, usedInvite.Body)
	accepted := f.call(t, http.MethodPost, f.siteAHost(), "/api/adty/teams/accept", fmt.Sprintf(`{"code":%q}`, used.Code), f.memberATok)
	require.Equal(t, http.StatusOK, accepted.Code, accepted.Body.String())
	teamAuthRejected(t, f.call(t, http.MethodPost, f.siteAHost(),
		fmt.Sprintf("/api/adty/teams/%d/invites/%d/revoke", created.Team.Id, used.Id), "", f.leaderATok))
	memberMine := f.call(t, http.MethodGet, f.siteAHost(), "/api/adty/teams/mine", "", f.memberATok)
	require.Equal(t, http.StatusOK, memberMine.Code, memberMine.Body.String())
	assert.Contains(t, memberMine.Body.String(), `"role":"member"`)

	listed := f.call(t, http.MethodGet, f.siteAHost(), fmt.Sprintf("/api/adty/teams/%d/invites", created.Team.Id), "", f.leaderATok)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	assert.Contains(t, listed.Body.String(), `"status":"revoked"`)
	assert.Contains(t, listed.Body.String(), `"status":"used"`)
	assert.Contains(t, listed.Body.String(), `"status":"outstanding"`)
	assert.NotContains(t, listed.Body.String(), "code_hash")
	assert.NotContains(t, listed.Body.String(), revocable.Code)
	assert.NotContains(t, listed.Body.String(), used.Code)
}
