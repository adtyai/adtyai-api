//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// A39/A08 Root-only writes: per-run operator ids scoped to this run.
var (
	runAdminID = 800000 + int(common.GetTimestamp()%100000)
	runRootID  = runAdminID + 100000
)

func roleCtxP(method, path, body string, params gin.Params, role, id int, username string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = r
	c.Params = params
	c.Set("id", id)
	c.Set("role", role)
	c.Set("username", username)
	return c, w
}

// rootCtx: Root operator (the only role allowed to mutate; no confirm needed).
func rootCtx(method, path, body string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	return roleCtxP(method, path, body, params, common.RoleRootUser, runRootID, "accept-root")
}

func respCode(w *httptest.ResponseRecorder) string {
	var r struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &r)
	return r.Code
}

func adminCtx(method, path, body string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = r
	c.Params = params
	c.Set("id", 2)
	c.Set("role", common.RoleAdminUser)
	c.Set("username", "accept-groupmeta-admin")
	return c, w
}

func parseData[T any](body []byte) T {
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    T      `json:"data"`
	}
	must(json.Unmarshal(body, &resp))
	if !resp.Success {
		panic("success=false: " + string(body))
	}
	return resp.Data
}

func findView(views []model.GroupMetaView, code string) (model.GroupMetaView, bool) {
	for _, v := range views {
		if v.Code == code {
			return v, true
		}
	}
	return model.GroupMetaView{}, false
}

func statusGroupMap() map[string]string {
	c, w := adminCtx(http.MethodGet, "/api/status", "", nil)
	// GetStatus does not need admin; reuse helper with guest-like ids
	c.Set("id", 0)
	c.Set("role", common.RoleGuestUser)
	controller.GetStatus(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("status want 200 got %d %s", w.Code, w.Body.String()))
	}
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			GroupDisplayNames map[string]string `json:"GroupDisplayNames"`
		} `json:"data"`
	}
	must(json.Unmarshal(w.Body.Bytes(), &resp))
	if !resp.Success {
		panic("status success=false")
	}
	if resp.Data.GroupDisplayNames == nil {
		resp.Data.GroupDisplayNames = map[string]string{}
	}
	return resp.Data.GroupDisplayNames
}

func main() {
	if os.Getenv("ADTY_SECRET_KEY") == "" {
		_ = os.Setenv("ADTY_SECRET_KEY", "adtyai-accept-test-secret-key-do-not-use-prod")
	}
	common.InitAdtySecretKeys()
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.IsMasterNode = true
	must(model.InitDB())
	must(model.InitLogDB())
	model.InitOptionMap()
	must(model.EnsureGroupMetaSchema(model.DB))
	fmt.Println("InitDB+group_meta OK")

	suffix := strings.ToLower(common.GetRandomString(6))
	codeA := "gm" + suffix // valid: [a-z0-9_-]{2,32}
	displayA := "AcceptGroup-" + suffix
	displayB := "RenamedGroup-" + suffix

	// Cleanup leftover from prior runs with same pattern not needed (unique suffix)

	// (0) A08 Root-only gating
	{
		body := fmt.Sprintf(`{"code":%q,"display_name":%q}`, "gz"+suffix, "Gate-"+suffix)
		c, w := roleCtxP(http.MethodPost, "/api/admin/groups", body, nil, common.RoleAdminUser, runAdminID, "accept-gm-admin")
		controller.AdminPostGroup(c)
		if w.Code != 403 || respCode(w) != "root_required" {
			panic(fmt.Sprintf("(0a) admin POST want 403 root_required got %d %s", w.Code, w.Body.String()))
		}
		c, w = roleCtxP(http.MethodPut, "/api/admin/groups/default", `{"display_name":"hack","confirm":true}`, gin.Params{{Key: "code", Value: "default"}}, common.RoleAdminUser, runAdminID, "accept-gm-admin")
		controller.AdminPutGroup(c)
		if w.Code != 403 || respCode(w) != "root_required" {
			panic(fmt.Sprintf("(0a) admin PUT want 403 got %d %s", w.Code, w.Body.String()))
		}
		var n int64
		_ = model.DB.Model(&model.GroupMeta{}).Where("code = ?", "gz"+suffix).Count(&n)
		if n != 0 {
			panic("(0c) rejected POST created a row")
		}
		fmt.Println("(0) admin POST/PUT → 403; no rows created OK")
	}

	// (1) POST create meta
	c, w := rootCtx(http.MethodPost, "/api/admin/groups",
		fmt.Sprintf(`{"code":%q,"display_name":%q,"sort":10}`, codeA, displayA), nil)
	controller.AdminPostGroup(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(1) POST want 200 got %d %s", w.Code, w.Body.String()))
	}
	created := parseData[model.GroupMetaView](w.Body.Bytes())
	if created.Code != codeA || created.DisplayName != displayA {
		panic(fmt.Sprintf("(1) create mismatch %+v", created))
	}
	fmt.Printf("(1) POST create %s → %s OK\n", codeA, displayA)

	// GET list contains it
	c, w = adminCtx(http.MethodGet, "/api/admin/groups", "", nil)
	controller.AdminGetGroups(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(1b) GET want 200 got %d %s", w.Code, w.Body.String()))
	}
	views := parseData[[]model.GroupMetaView](w.Body.Bytes())
	v, ok := findView(views, codeA)
	if !ok || v.DisplayName != displayA {
		panic(fmt.Sprintf("(1b) GET missing/wrong %+v ok=%v", v, ok))
	}
	fmt.Println("(1b) GET list (Admin) shows display_name OK")

	// PUT rename display_name
	c, w = rootCtx(http.MethodPut, "/api/admin/groups/"+codeA,
		fmt.Sprintf(`{"display_name":%q}`, displayB),
		gin.Params{{Key: "code", Value: codeA}})
	controller.AdminPutGroup(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(1c) PUT rename want 200 got %d %s", w.Code, w.Body.String()))
	}
	renamed := parseData[model.GroupMetaView](w.Body.Bytes())
	if renamed.DisplayName != displayB || renamed.Code != codeA {
		panic(fmt.Sprintf("(1c) rename mismatch %+v", renamed))
	}
	fmt.Printf("(1c) PUT display_name → %s OK\n", displayB)

	sm := statusGroupMap()
	if sm[codeA] != displayB {
		panic(fmt.Sprintf("(1d) status GroupDisplayNames[%s] want %s got %v", codeA, displayB, sm[codeA]))
	}
	fmt.Println("(1d) status GroupDisplayNames OK")

	// (2) PUT try change code → 400
	c, w = rootCtx(http.MethodPut, "/api/admin/groups/"+codeA,
		fmt.Sprintf(`{"code":%q,"display_name":%q}`, "other"+suffix, displayB),
		gin.Params{{Key: "code", Value: codeA}})
	controller.AdminPutGroup(c)
	if w.Code != 400 {
		panic(fmt.Sprintf("(2) PUT change code want 400 got %d %s", w.Code, w.Body.String()))
	}
	var errResp struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	must(json.Unmarshal(w.Body.Bytes(), &errResp))
	if errResp.Success || errResp.Code != "code_immutable" {
		panic(fmt.Sprintf("(2) want code_immutable got %+v body=%s", errResp, w.Body.String()))
	}
	fmt.Println("(2) PUT body code ≠ path → 400 code_immutable OK")

	// Confirm path code unchanged in DB
	var row model.GroupMeta
	must(model.DB.Where("code = ?", codeA).First(&row).Error)
	if row.DisplayName != displayB {
		panic("(2b) display_name changed unexpectedly after rejected code change")
	}
	var wrong int64
	_ = model.DB.Model(&model.GroupMeta{}).Where("code = ?", "other"+suffix).Count(&wrong)
	if wrong != 0 {
		panic("(2b) rogue code row created")
	}
	fmt.Println("(2b) code unchanged in DB OK")

	// (3) rename does not affect token.group / GroupRatio key
	u := model.User{
		Username:    "accept-gm-" + suffix,
		Password:    "unused",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       codeA,
		AuthVersion: 1,
		Quota:       1000,
		AffCode:     "gm" + suffix,
	}
	must(model.DB.Create(&u).Error)

	tok := model.Token{
		UserId:         u.Id,
		Key:            "gk-" + suffix + common.GetRandomString(16),
		Status:         1,
		Name:           "accept-gm-" + suffix,
		CreatedTime:    common.GetTimestamp(),
		AccessedTime:   common.GetTimestamp(),
		ExpiredTime:    -1,
		RemainQuota:    100,
		UnlimitedQuota: true,
		Group:          codeA,
	}
	must(model.DB.Create(&tok).Error)

	// Ensure GroupRatio has this code so ratio lookup is by code
	prevRatio := ratio_setting.GroupRatio2JSONString()
	ratioMap := ratio_setting.GetGroupRatioCopy()
	ratioMap[codeA] = 1.5
	b, _ := json.Marshal(ratioMap)
	must(ratio_setting.UpdateGroupRatioByJSONString(string(b)))
	defer func() { _ = ratio_setting.UpdateGroupRatioByJSONString(prevRatio) }()

	ratioBefore := ratio_setting.GetGroupRatio(codeA)

	c, w = rootCtx(http.MethodPut, "/api/admin/groups/"+codeA,
		fmt.Sprintf(`{"display_name":%q,"sort":20}`, "FinalName-"+suffix),
		gin.Params{{Key: "code", Value: codeA}})
	controller.AdminPutGroup(c)
	if w.Code != 200 {
		panic(fmt.Sprintf("(3) PUT final rename want 200 got %d %s", w.Code, w.Body.String()))
	}

	var tok2 model.Token
	must(model.DB.Where("id = ?", tok.Id).First(&tok2).Error)
	if tok2.Group != codeA {
		panic(fmt.Sprintf("(3) token.group want %s got %s", codeA, tok2.Group))
	}
	var u2 model.User
	must(model.DB.Where("id = ?", u.Id).First(&u2).Error)
	if u2.Group != codeA {
		panic(fmt.Sprintf("(3) user.group want %s got %s", codeA, u2.Group))
	}
	ratioAfter := ratio_setting.GetGroupRatio(codeA)
	if ratioAfter != ratioBefore || ratioAfter != 1.5 {
		panic(fmt.Sprintf("(3) GroupRatio(%s) want 1.5 got before=%v after=%v", codeA, ratioBefore, ratioAfter))
	}
	fmt.Println("(3) rename does not change token.group / user.group / GroupRatio key OK")

	// (4) illegal code POST → 400
	for _, bad := range []string{"A", "BadCode", "x", "has space", "UPPER", ""} {
		c, w = rootCtx(http.MethodPost, "/api/admin/groups",
			fmt.Sprintf(`{"code":%q,"display_name":"x"}`, bad), nil)
		controller.AdminPostGroup(c)
		if w.Code != 400 {
			panic(fmt.Sprintf("(4) bad code %q want 400 got %d %s", bad, w.Code, w.Body.String()))
		}
	}
	fmt.Println("(4) illegal code POST → 400 OK")

	// Cleanup accept rows (keep shared cloud PG tidy)
	_ = model.DB.Where("code = ?", codeA).Delete(&model.GroupMeta{})
	_ = model.DB.Where("id = ?", tok.Id).Delete(&model.Token{})
	_ = model.DB.Where("id = ?", u.Id).Delete(&model.User{})
	fmt.Println("cleaned accept rows")

	fmt.Println("ALL accept_group_meta checks passed")
}
