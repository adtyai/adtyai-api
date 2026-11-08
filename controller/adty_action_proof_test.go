package controller

import (
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdtyAdminProofCatalogDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			if dialect == "postgres" && os.Getenv("TEST_POSTGRES_DSN") == "" {
				t.Skip("set TEST_POSTGRES_DSN")
			}
			t.Setenv("TEST_SECURITY_DIALECT", dialect)
			operator, _, _ := setupAdminUserTest(t)
			setupMetadataActionPermissions(t, model.DB)
			for _, scope := range []struct{ proof, required string }{
				{"admin.user.create", "adty_user:create"}, {"admin.user.delete", "adty_user:delete"},
				{"admin.user.update", "adty_user:write"}, {"admin.user.manage", "adty_user:write"},
				{"admin.user.passkey.reset", "adty_user:write"}, {"admin.user.2fa.disable", "adty_user:write"}, {"admin.user.binding.clear", "adty_user:write"},
			} {
				for _, grant := range []struct {
					name, scope string
					version     int
					allow       bool
				}{
					{"old0", "user:write", 0, true}, {"new2 old scope", "user:write", 2, false}, {"new2 independent scope", scope.required, 2, true}, {"unknown", scope.required, 99, false},
				} {
					t.Run(scope.proof+"/"+grant.name, func(t *testing.T) {
						_, token := createScopedAccessToken(t, operator.Id, 0, grant.scope)
						require.NoError(t, model.DB.Model(token).Update("scope_version", grant.version).Error)
						t.Cleanup(func() { require.NoError(t, model.DB.Delete(token).Error) })
						identity := service.AuthIdentity{UserID: operator.Id, SessionID: model.AccessTokenSessionID(token.Id), UserAuthVersion: operator.AuthVersion, SessionVersion: model.AccessTokenSessionVersion}
						_, err := service.GetVerificationRequirements(identity, scope.proof)
						if grant.allow {
							assert.NoError(t, err)
						} else {
							assert.ErrorIs(t, err, service.ErrVerificationForbidden)
						}
					})
				}
			}
		})
	}
}

func TestAdtyAdminProofRechecksLiveAndPATGrantsDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			if dialect == "postgres" && os.Getenv("TEST_POSTGRES_DSN") == "" {
				t.Skip("set TEST_POSTGRES_DSN")
			}
			t.Setenv("TEST_SECURITY_DIALECT", dialect)
			operator, _, target := setupAdminUserTest(t)
			setupMetadataActionPermissions(t, model.DB)
			require.NoError(t, model.DB.Model(operator).Update("role", common.RoleAdminUser).Error)
			require.NoError(t, model.PublishUserAuthCache(operator.Id))
			raw, token := createScopedAccessToken(t, operator.Id, 0, "adty_user:delete")
			identity := service.AuthIdentity{UserID: operator.Id, SessionID: model.AccessTokenSessionID(token.Id), UserAuthVersion: operator.AuthVersion, SessionVersion: model.AccessTokenSessionVersion}
			operation := service.VerificationOperation{Scope: service.VerificationScopeAdminUserDelete, Context: []byte(fmt.Sprintf(`{"user_id":%d}`, target.Id))}
			binding, err := service.BindVerificationOperation(operation)
			require.NoError(t, err)
			_, err = service.GetVerificationRequirements(identity, operation.Scope)
			assert.ErrorIs(t, err, service.ErrVerificationForbidden, "matching token scope cannot replace live delete permission")
			require.NoError(t, authz.SetUserPermissions(operator.Id, authz.PermissionsMap{"adty_user": {"delete": true}}))
			router := newAccessTokenTestRouter()
			body := fmt.Sprintf(`{"scope":"admin.user.delete","method":"password","password":"enrollment-password","context":{"user_id":%d}}`, target.Id)
			response := accessTokenRequest(router, http.MethodPost, "/api/verify", raw, "", body)
			verified := decodeSecurityEnrollmentResponse(t, response)
			require.True(t, verified.Success, response.Body.String())
			var proof service.SecurityProof
			require.NoError(t, common.Unmarshal(verified.Data, &proof))
			require.NoError(t, authz.SetUserPermissions(operator.Id, authz.PermissionsMap{"adty_user": {"delete": false}}))
			_, err = service.GetVerificationRequirements(identity, operation.Scope)
			assert.ErrorIs(t, err, service.ErrVerificationForbidden)
			_, err = service.CompleteSecurityVerification(identity, binding, service.VerificationMethodPassword)
			assert.ErrorIs(t, err, service.ErrVerificationForbidden)
			_, err = service.ConsumeOperationProof(proof.ProofToken, identity, operation)
			assert.ErrorIs(t, err, service.ErrVerificationForbidden)
			require.NoError(t, authz.SetUserPermissions(operator.Id, authz.PermissionsMap{"adty_user": {"delete": true}}))
			_, err = model.UpdateUserAccessToken(operator.Id, token.Id, "scope revoked", []string{"adty_user:read"})
			require.NoError(t, err)
			_, err = service.GetVerificationRequirements(identity, operation.Scope)
			assert.ErrorIs(t, err, service.ErrVerificationForbidden)
			_, err = service.CompleteSecurityVerification(identity, binding, service.VerificationMethodPassword)
			assert.ErrorIs(t, err, service.ErrVerificationForbidden)
			_, err = service.ConsumeOperationProof(proof.ProofToken, identity, operation)
			assert.ErrorIs(t, err, service.ErrVerificationForbidden)
			_, err = model.UpdateUserAccessToken(operator.Id, token.Id, "scope restored", []string{"adty_user:delete"})
			require.NoError(t, err)
			_, err = service.ConsumeOperationProof(proof.ProofToken, identity, operation)
			require.NoError(t, err, "rejected consumption did not spend the bound proof")
			_, err = service.ConsumeOperationProof(proof.ProofToken, identity, operation)
			assert.ErrorIs(t, err, service.ErrProofConsumed)
		})
	}
}

func TestAdtyDeletePATUsesOwnBoundProofDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			if dialect == "postgres" && os.Getenv("TEST_POSTGRES_DSN") == "" {
				t.Skip("set TEST_POSTGRES_DSN")
			}
			t.Setenv("TEST_SECURITY_DIALECT", dialect)
			for _, grant := range []struct {
				name, scope string
				version     int
				allow       bool
			}{{"legacy0", "user:write", 0, true}, {"new2delete", "adty_user:delete", 2, true}, {"new2oldscope", "user:write", 2, false}} {
				t.Run(grant.name, func(t *testing.T) {
					operator, _, target := setupAdminUserTest(t)
					setupMetadataActionPermissions(t, model.DB)
					require.NoError(t, model.DB.AutoMigrate(&model.Token{}))
					raw, token := createScopedAccessToken(t, operator.Id, 0, grant.scope)
					require.NoError(t, model.DB.Model(token).Update("scope_version", grant.version).Error)
					router := newAccessTokenTestRouter()
					body := fmt.Sprintf(`{"scope":"admin.user.delete","method":"password","password":"enrollment-password","context":{"user_id":%d}}`, target.Id)
					response := accessTokenRequest(router, http.MethodPost, "/api/verify", raw, "", body)
					verified := decodeSecurityEnrollmentResponse(t, response)
					if !grant.allow {
						assert.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
						assert.False(t, verified.Success)
						_, err := model.GetUserById(target.Id, false)
						assert.NoError(t, err)
						return
					}
					require.True(t, verified.Success, response.Body.String())
					var proof service.SecurityProof
					require.NoError(t, common.Unmarshal(verified.Data, &proof))
					response = accessTokenRequest(router, http.MethodDelete, fmt.Sprintf("/api/user/%d", target.Id), raw, proof.ProofToken, "")
					result := decodeSecurityEnrollmentResponse(t, response)
					require.True(t, result.Success, response.Body.String())
					_, err := model.GetUserById(target.Id, true)
					assert.Error(t, err)
					var audit model.AuditLog
					require.NoError(t, model.LOG_DB.Where("action = ?", "user.delete").Last(&audit).Error)
					encoded, err := common.Marshal(audit)
					require.NoError(t, err)
					for _, secret := range []string{raw, proof.ProofToken, "enrollment-password"} {
						assert.NotContains(t, string(encoded), secret)
					}
				})
			}
		})
	}
}
