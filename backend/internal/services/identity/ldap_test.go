package identity

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/contracts"
	"github.com/linskybing/nexuspaas/backend/internal/platform"

	"github.com/go-ldap/ldap/v3"
)

func TestLDAPAuthFirstLocalFallbackAndLocalState(t *testing.T) {
	fake := newFakeLDAPDirectory()
	fake.passwords["alice"] = "ldap-password"
	fake.passwords["disabled"] = "ldap-password"
	app := newLDAPIdentityTestApp(t, fake)
	seedLDAPIdentityUser(t, app, "US1", "alice", "local-password", "offline")
	seedLDAPIdentityUser(t, app, "US2", "disabled", "local-password", "disabled")

	code, data, _ := login(app, identityRequest(http.MethodPost, pathLogin, `{"username":"alice","password":"ldap-password"}`), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	if fake.authAttempts["alice"] != 1 {
		t.Fatalf("ldap auth attempts = %#v, want alice attempted first", fake.authAttempts)
	}

	code, data, _ = login(app, identityRequest(http.MethodPost, pathLogin, `{"username":"alice","password":"local-password"}`), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)

	code, data, _ = login(app, identityRequest(http.MethodPost, pathLogin, `{"username":"disabled","password":"ldap-password"}`), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusUnauthorized)
}

func TestLDAPCreateCompensatesWhenLocalCreateFails(t *testing.T) {
	fake := newFakeLDAPDirectory()
	base := platform.NewStore()
	app := platform.NewApp(ldapTestConfig(), platform.WithStore(&failingIdentityStore{RecordStore: base, failCreateUsers: true}))
	Register(app)
	withFakeLDAPDirectory(t, fake)

	code, data, _ := register(app, identityRequest(http.MethodPost, "/api/v1/register", `{"username":"alice","password":"correct-password"}`), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusInternalServerError)
	if len(fake.upserts) != 1 || fake.upserts[0] != "alice" {
		t.Fatalf("ldap upserts = %v, want alice before local create", fake.upserts)
	}
	if len(fake.restoredUpserts) != 1 || fake.restoredUpserts[0].Username != "alice" || !fake.restoredUpserts[0].Created {
		t.Fatalf("ldap restore = %#v, want created alice compensation", fake.restoredUpserts)
	}
	if got := len(app.Store.List(context.Background(), usersResource)); got != 0 {
		t.Fatalf("local users = %d, want none after create failure", got)
	}
	if got := len(app.Events.Outbox()); got != 0 {
		t.Fatalf("events = %d, want none on compensated create failure", got)
	}
}

func TestLDAPCreateExistingEntryCompensationPreservesLDAPPassword(t *testing.T) {
	fake := newFakeLDAPDirectory()
	fake.usernames["alice"] = true
	fake.passwords["alice"] = "existing-password"
	base := platform.NewStore()
	app := platform.NewApp(ldapTestConfig(), platform.WithStore(&failingIdentityStore{RecordStore: base, failCreateUsers: true}))
	Register(app)
	withFakeLDAPDirectory(t, fake)

	code, data, _ := register(app, identityRequest(http.MethodPost, "/api/v1/register", `{"username":"alice","password":"new-password"}`), platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusInternalServerError)
	if fake.passwords["alice"] != "existing-password" {
		t.Fatalf("existing LDAP password changed to %q", fake.passwords["alice"])
	}
	if len(fake.restoredUpserts) != 1 || fake.restoredUpserts[0].Created {
		t.Fatalf("ldap restore = %#v, want existing-entry compensation", fake.restoredUpserts)
	}
	if containsString(fake.restoredUpserts[0].ModifiedAttrs, "userPassword") {
		t.Fatalf("modified attrs = %v, userPassword should not be touched before local create", fake.restoredUpserts[0].ModifiedAttrs)
	}
}

func TestLDAPUpdateCompensatesWhenLocalUpdateFails(t *testing.T) {
	fake := newFakeLDAPDirectory()
	fake.previousAttrs["alice"] = map[string][]string{"cn": {"Alice Old"}, "gidNumber": {"5002"}}
	base := platform.NewStore()
	seedStoreIdentityUser(t, base, "US1", "alice", "old-password", "online")
	app := platform.NewApp(ldapTestConfig(), platform.WithStore(&failingIdentityStore{RecordStore: base, failUpdateUsers: true}))
	Register(app)
	withFakeLDAPDirectory(t, fake)
	seedLDAPIdentityUser(t, app, "ADMIN", "admin", "admin-password", "online")
	_, _ = app.Store.Update(context.Background(), usersResource, "ADMIN", map[string]any{"role": "admin", "system_role": 0})

	req := identityUserRequest(http.MethodPut, "/api/v1/users/US1", `{"name":"Alice Updated"}`, "ADMIN")
	req.SetPathValue("id", "US1")
	code, data, _ := updateUser(app, req, platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusServiceUnavailable)
	if len(fake.restoredUpserts) != 1 || fake.restoredUpserts[0].Username != "alice" {
		t.Fatalf("ldap restore = %#v, want alice restore after local update failure", fake.restoredUpserts)
	}
	if !containsString(fake.restoredUpserts[0].ModifiedAttrs, "mail") {
		t.Fatalf("restore modified attrs = %v, want mail tracked for absent-attribute deletion", fake.restoredUpserts[0].ModifiedAttrs)
	}
	if _, ok := fake.restoredUpserts[0].Previous["mail"]; ok {
		t.Fatalf("restore previous attrs = %#v, mail should be absent before attempted update", fake.restoredUpserts[0].Previous)
	}
	if got := len(app.Events.Outbox()); got != 0 {
		t.Fatalf("events = %d, want none on compensated update failure", got)
	}
}

func TestLDAPPasswordRollbackWhenLDAPSyncFails(t *testing.T) {
	fake := newFakeLDAPDirectory()
	fake.upsertErr["alice"] = errors.New("directory down")
	app := newLDAPIdentityTestApp(t, fake)
	seedLDAPIdentityUser(t, app, "ADMIN", "admin", "admin-password", "online")
	_, _ = app.Store.Update(context.Background(), usersResource, "ADMIN", map[string]any{"role": "admin", "system_role": 0})
	seedLDAPIdentityUser(t, app, "US1", "alice", "old-password", "online")

	req := identityUserRequest(http.MethodPut, "/api/v1/users/batch/password", `{"ids":["US1"],"password":"new-password"}`, "ADMIN")
	code, data, _ := batchResetPassword(app, req, platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	if result := data.(map[string]any); result["succeeded"] != 0 || result["failed"] != 1 {
		t.Fatalf("batch password result = %#v, want one failed item", result)
	}
	user, _ := app.Store.Get(context.Background(), usersResource, "US1")
	if !platform.VerifySecret(textValue(user.Data, "password_hash"), "old-password") || platform.VerifySecret(textValue(user.Data, "password_hash"), "new-password") {
		t.Fatalf("password hash was not rolled back: %#v", user.Data["password_hash"])
	}
	if got := len(app.Events.Outbox()); got != 0 {
		t.Fatalf("events = %d, want none on failed password sync", got)
	}
}

func TestLDAPDeleteRestoresDirectoryWhenLocalDeleteFails(t *testing.T) {
	fake := newFakeLDAPDirectory()
	base := platform.NewStore()
	seedStoreIdentityUser(t, base, "ADMIN", "admin", "admin-password", "online")
	_, _ = base.Update(context.Background(), usersResource, "ADMIN", map[string]any{"role": "admin", "system_role": 0})
	seedStoreIdentityUser(t, base, "US1", "alice", "old-password", "online")
	app := platform.NewApp(ldapTestConfig(), platform.WithStore(&failingIdentityStore{RecordStore: base, failDeleteUsers: true}))
	Register(app)
	withFakeLDAPDirectory(t, fake)

	req := identityUserRequest(http.MethodDelete, "/api/v1/users/US1", "", "ADMIN")
	req.SetPathValue("id", "US1")
	code, data, _ := deleteUser(app, req, platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusServiceUnavailable)
	if len(fake.deletes) != 1 || fake.deletes[0] != "alice" {
		t.Fatalf("ldap deletes = %v, want alice delete attempted first", fake.deletes)
	}
	if len(fake.restoredDeleted) != 1 || fake.restoredDeleted[0] != "alice" {
		t.Fatalf("ldap restored deletes = %v, want alice re-upsert compensation", fake.restoredDeleted)
	}
	if _, ok := app.Store.Get(context.Background(), usersResource, "US1"); !ok {
		t.Fatal("local user was deleted despite local delete failure")
	}
}

func TestLDAPLifecycleSuccessPublishesAfterLocalAndLDAPSuccess(t *testing.T) {
	fake := newFakeLDAPDirectory()
	app := newLDAPIdentityTestApp(t, fake)
	seedLDAPIdentityUser(t, app, "ADMIN", "admin", "admin-password", "online")
	_, _ = app.Store.Update(context.Background(), usersResource, "ADMIN", map[string]any{"role": "admin", "system_role": 0})
	seedLDAPIdentityUser(t, app, "US1", "alice", "old-password", "online")

	updateReq := identityUserRequest(http.MethodPut, "/api/v1/users/US1", `{"name":"Alice Updated","email":"alice.updated@example.org"}`, "ADMIN")
	updateReq.Header.Set("X-Trace-ID", "trace-ldap-success")
	updateReq.Header.Set("Idempotency-Key", "idem-ldap-update")
	updateReq.SetPathValue("id", "US1")
	code, data, _ := updateUser(app, updateReq, platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	updated, _ := app.Store.Get(context.Background(), usersResource, "US1")
	if updated.Data["name"] != "Alice Updated" || updated.Data["email"] != "alice.updated@example.org" {
		t.Fatalf("updated user = %#v", updated.Data)
	}
	if len(fake.upserts) != 1 || fake.upserts[0] != "alice" {
		t.Fatalf("ldap upserts = %v, want alice update", fake.upserts)
	}
	if lastIdentityEvent(app, "UserUpdated") == nil {
		t.Fatal("missing UserUpdated event after successful LDAP/local update")
	}

	deleteReq := identityUserRequest(http.MethodDelete, "/api/v1/users/US1", "", "ADMIN")
	deleteReq.Header.Set("X-Trace-ID", "trace-ldap-success")
	deleteReq.Header.Set("Idempotency-Key", "idem-ldap-delete")
	deleteReq.SetPathValue("id", "US1")
	code, data, _ = deleteUser(app, deleteReq, platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	if _, ok := app.Store.Get(context.Background(), usersResource, "US1"); ok {
		t.Fatal("user still exists after successful delete")
	}
	if len(fake.deletes) != 1 || fake.deletes[0] != "alice" {
		t.Fatalf("ldap deletes = %v, want alice delete", fake.deletes)
	}
	if lastIdentityEvent(app, "UserDisabled") == nil {
		t.Fatal("missing UserDisabled event after successful LDAP/local delete")
	}
}

func TestLDAPBatchCreateUsesPerItemSemantics(t *testing.T) {
	fake := newFakeLDAPDirectory()
	fake.upsertErr["bad"] = errors.New("directory rejected user")
	app := newLDAPIdentityTestApp(t, fake)
	seedLDAPIdentityUser(t, app, "ADMIN", "admin", "admin-password", "online")
	_, _ = app.Store.Update(context.Background(), usersResource, "ADMIN", map[string]any{"role": "admin", "system_role": 0})

	req := identityUserRequest(http.MethodPost, "/api/v1/users/batch", `{"users":[{"username":"good","password":"correct-password"},{"username":"bad","password":"correct-password"}]}`, "ADMIN")
	code, data, _ := batchCreateUsers(app, req, platform.RouteSpec{})
	assertIdentityStatus(t, code, data, http.StatusOK)
	result := data.(map[string]any)
	if result["succeeded"] != 1 || result["failed"] != 1 {
		t.Fatalf("batch create result = %#v, want mixed success/failure", result)
	}
	if _, ok := findUserByUsername(app, identityRequest(http.MethodGet, "/", ""), "good"); !ok {
		t.Fatal("good user missing after successful batch item")
	}
	if _, ok := findUserByUsername(app, identityRequest(http.MethodGet, "/", ""), "bad"); ok {
		t.Fatal("bad user was locally created despite LDAP failure")
	}
}

func TestLDAPMirrorSyncEnsuresEligibleDBUsers(t *testing.T) {
	fake := newFakeLDAPDirectory()
	fake.usernames["existing"] = true
	app := newLDAPIdentityTestApp(t, fake)
	seedLDAPIdentityUser(t, app, "US1", "existing", "password", "online")
	seedLDAPIdentityUser(t, app, "US2", "missing", "password", "offline")
	seedLDAPIdentityUser(t, app, "US3", "deleted", "password", "deleted")
	if _, err := app.Store.Create(context.Background(), usersResource, map[string]any{"id": "US4", "username": "", "status": "online"}); err != nil {
		t.Fatal(err)
	}

	if err := syncLDAPMirror(context.Background(), app, fake); err != nil {
		t.Fatalf("syncLDAPMirror() error = %v", err)
	}
	if !fake.usernames["missing"] {
		t.Fatalf("mirror usernames = %#v, want missing user upserted", fake.usernames)
	}
	if fake.usernames["deleted"] || fake.usernames[""] {
		t.Fatalf("mirror synced ineligible users: %#v", fake.usernames)
	}
}

func TestLDAPHelpersWithoutLDAPUseLocalStore(t *testing.T) {
	app := platform.NewApp(platform.Config{ServiceName: serviceName, RequireAuth: true})
	Register(app)
	seedLDAPIdentityUser(t, app, "US1", "alice", "old-password", "online")
	req := identityRequest(http.MethodPut, "/api/v1/users/US1", "")

	updated, status, data := updateUserWithLDAP(app, req, "US1", nil, map[string]any{"name": "Alice Local"})
	if status != http.StatusOK || data != nil || updated.Data["name"] != "Alice Local" {
		t.Fatalf("update without LDAP = status %d data %#v record %#v", status, data, updated.Data)
	}
	if resetUserPasswordWithLDAP(app, req, "missing", "new-password") {
		t.Fatal("resetUserPasswordWithLDAP missing user succeeded")
	}
	if !resetUserPasswordWithLDAP(app, req, "US1", "new-password") {
		t.Fatal("resetUserPasswordWithLDAP without LDAP failed")
	}
	if _, ok := updateUserRoleWithLDAP(app, req, "missing", map[string]any{"role": "manager"}); ok {
		t.Fatal("updateUserRoleWithLDAP missing user succeeded")
	}
	roleUpdated, ok := updateUserRoleWithLDAP(app, req, "US1", map[string]any{"role": "manager", "system_role": 1})
	if !ok || roleUpdated["role"] != "manager" || roleUpdated["system_role"] != 1 {
		t.Fatalf("role update without LDAP = ok %v data %#v", ok, roleUpdated)
	}
	status, data = deleteUserWithLDAP(app, req, "missing")
	if status != http.StatusNotFound || textValue(data, "message") != msgUserNotFound {
		t.Fatalf("delete missing without LDAP = status %d data %#v", status, data)
	}
	status, data = deleteUserWithLDAP(app, req, "US1")
	if status != http.StatusOK || data != nil {
		t.Fatalf("delete without LDAP = status %d data %#v", status, data)
	}
}

func TestLDAPPasswordAndRoleSuccessSyncDirectory(t *testing.T) {
	fake := newFakeLDAPDirectory()
	app := newLDAPIdentityTestApp(t, fake)
	seedLDAPIdentityUser(t, app, "US1", "alice", "old-password", "online")
	req := identityRequest(http.MethodPut, "/api/v1/users/US1", "")

	payload := map[string]any{"password": "new-password"}
	update := map[string]any{"password_hash": platform.HashSecret("new-password")}
	updated, status, data := updateUserWithLDAP(app, req, "US1", payload, update)
	if status != http.StatusOK || data != nil || !platform.VerifySecret(textValue(updated.Data, "password_hash"), "new-password") {
		t.Fatalf("password update = status %d data %#v record %#v", status, data, updated.Data)
	}
	if fake.passwords["alice"] != "new-password" {
		t.Fatalf("LDAP password = %q, want synchronized", fake.passwords["alice"])
	}
	roleUpdated, ok := updateUserRoleWithLDAP(app, req, "US1", map[string]any{"role": "manager", "system_role": 1})
	if !ok || roleUpdated["role"] != "manager" || len(fake.upserts) < 2 {
		t.Fatalf("role update = ok %v data %#v upserts %v", ok, roleUpdated, fake.upserts)
	}
}

func TestLDAPMirrorSyncHandlesDisabledAndErrors(t *testing.T) {
	if err := syncLDAPMirror(context.Background(), nil, nil); err != nil {
		t.Fatalf("disabled nil app mirror error = %v", err)
	}
	if dir, ok := ldapDirectoryFor(nil); ok || dir != nil {
		t.Fatalf("nil app ldap directory = %#v/%v, want disabled", dir, ok)
	}
	if sanitizeLDAPError(nil) != "" {
		t.Fatal("nil LDAP error should sanitize to empty string")
	}
	compensateLDAPUpsert(context.Background(), nil, ldapUpsertResult{Username: "alice"})
	disabledApp := platform.NewApp(platform.Config{ServiceName: serviceName})
	if err := syncLDAPMirror(context.Background(), disabledApp, nil); err != nil {
		t.Fatalf("disabled mirror error = %v", err)
	}
	previousFactory := newLDAPDirectory
	newLDAPDirectory = func(platform.Config) ldapDirectory { return nil }
	t.Cleanup(func() { newLDAPDirectory = previousFactory })
	enabledApp := platform.NewApp(ldapTestConfig())
	if err := syncLDAPMirror(context.Background(), enabledApp, nil); !errors.Is(err, errLDAPNotConfigured) {
		t.Fatalf("missing LDAP directory error = %v, want not configured", err)
	}

	fake := newFakeLDAPDirectory()
	fake.listErr = errors.New("directory list failed")
	app := newLDAPIdentityTestApp(t, fake)
	if err := syncLDAPMirror(context.Background(), app, fake); !errors.Is(err, fake.listErr) {
		t.Fatalf("mirror list error = %v, want %v", err, fake.listErr)
	}

	failingUpsert := newFakeLDAPDirectory()
	failingUpsert.upsertErr["alice"] = errors.New("directory write failed")
	app = newLDAPIdentityTestApp(t, failingUpsert)
	seedLDAPIdentityUser(t, app, "US1", "alice", "password", "online")
	if err := syncLDAPMirror(context.Background(), app, failingUpsert); !errors.Is(err, failingUpsert.upsertErr["alice"]) {
		t.Fatalf("mirror upsert error = %v, want %v", err, failingUpsert.upsertErr["alice"])
	}
}

func TestGoLDAPDirectoryBuildsEscapedFilterAndDN(t *testing.T) {
	dir := newGoLDAPDirectory(platform.Config{
		LDAPHost:           "ldap.local",
		LDAPPort:           1389,
		LDAPUserSearchBase: "ou=users,dc=example,dc=org",
		LDAPUserFilter:     "(&(uid=%s)(objectClass=inetOrgPerson))",
	})
	filter, err := dir.userFilter(`a*b(c)\d`)
	if err != nil {
		t.Fatalf("userFilter() error = %v", err)
	}
	if !strings.Contains(filter, `\2a`) || !strings.Contains(filter, `\28`) || !strings.Contains(filter, `\29`) || !strings.Contains(filter, `\5c`) {
		t.Fatalf("filter = %q, want escaped LDAP filter chars", filter)
	}
	if dn := dir.userDN(`a,b+c`); strings.Contains(dn, "uid=a,b+c") || !strings.Contains(dn, `\`) {
		t.Fatalf("DN = %q, want escaped username component", dn)
	}
}

func TestGoLDAPDirectoryTimeoutsAndFilterValidation(t *testing.T) {
	defaultDir := newGoLDAPDirectory(platform.Config{})
	if defaultDir.timeout() != 2*time.Second {
		t.Fatalf("default timeout = %v, want 2s", defaultDir.timeout())
	}
	dir := newGoLDAPDirectory(platform.Config{
		LDAPUserFilter: "(uid=%s)",
		AdapterTimeout: 10 * time.Millisecond,
	})
	if dir.timeout() != 10*time.Millisecond || dir.searchTimeLimit() != 1 {
		t.Fatalf("timeout/search limit = %v/%d", dir.timeout(), dir.searchTimeLimit())
	}
	if _, err := dir.userFilter("alice"); err != nil {
		t.Fatalf("userFilter valid config error = %v", err)
	}
	badFilter := newGoLDAPDirectory(platform.Config{LDAPUserFilter: "(uid=alice)"})
	if _, err := badFilter.userFilter("alice"); !errors.Is(err, errLDAPNotConfigured) {
		t.Fatalf("userFilter invalid config error = %v, want not configured", err)
	}
}

func TestGoLDAPDirectoryFailFastBranches(t *testing.T) {
	dir := testGoLDAPDirectory()
	if _, err := dir.Authenticate(context.Background(), "", "password"); !errors.Is(err, errLDAPInvalidCredentials) {
		t.Fatalf("Authenticate empty username error = %v, want invalid credentials", err)
	}
	if _, err := dir.UpsertUser(context.Background(), map[string]any{}, ""); !errors.Is(err, errLDAPNotConfigured) {
		t.Fatalf("UpsertUser empty username error = %v, want not configured", err)
	}
	if err := dir.RestoreUpsert(context.Background(), ldapUpsertResult{}); err != nil {
		t.Fatalf("RestoreUpsert empty result error = %v", err)
	}
	if err := dir.DeleteUser(context.Background(), map[string]any{}); err != nil {
		t.Fatalf("DeleteUser empty username error = %v", err)
	}
	if err := normalizeLDAPDeleteError(nil); err != nil {
		t.Fatalf("normalizeLDAPDeleteError(nil) = %v", err)
	}
	if err := normalizeLDAPDeleteError(ldap.NewError(ldap.LDAPResultNoSuchObject, errors.New("missing"))); err != nil {
		t.Fatalf("normalizeLDAPDeleteError(no such object) = %v", err)
	}
}

func TestGoLDAPDirectoryCanceledContextBranches(t *testing.T) {
	dir := testGoLDAPDirectory()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := dir.Authenticate(ctx, "alice", "password"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Authenticate canceled error = %v, want context canceled", err)
	}
	if _, err := dir.UpsertUser(ctx, map[string]any{"username": "alice"}, "password"); !errors.Is(err, context.Canceled) {
		t.Fatalf("UpsertUser canceled error = %v, want context canceled", err)
	}
	if err := dir.RestoreUpsert(ctx, ldapUpsertResult{Username: "alice", DN: "uid=alice,ou=users,dc=example,dc=org"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("RestoreUpsert canceled error = %v, want context canceled", err)
	}
	if err := dir.DeleteUser(ctx, map[string]any{"username": "alice"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("DeleteUser canceled error = %v, want context canceled", err)
	}
	if _, err := dir.dial(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("dial canceled error = %v, want context canceled", err)
	}
	if _, err := dir.adminConn(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("adminConn canceled error = %v, want context canceled", err)
	}
	if _, err := dir.findUser(ctx, nil, "alice", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("findUser canceled error = %v, want context canceled", err)
	}
	if _, err := dir.ListUsernames(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("ListUsernames canceled error = %v, want context canceled", err)
	}
	if err := dir.RestoreDeletedUser(ctx, map[string]any{}); !errors.Is(err, errLDAPNotConfigured) {
		t.Fatalf("RestoreDeletedUser empty user error = %v, want not configured", err)
	}
	if err := dir.RestoreDeletedUser(ctx, map[string]any{"username": "alice"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("RestoreDeletedUser canceled user error = %v, want context canceled", err)
	}
}

func TestGoLDAPDirectoryAttributeAndSnapshotBranches(t *testing.T) {
	dir := testGoLDAPDirectory()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	user := map[string]any{"username": "alice", "full_name": "Alice Example", "email": "alice@example.org", "system_role": 1}
	attrs := dir.userAddAttributes(ctx, nil, user, "secret")
	if len(attrs) == 0 || attrsByName(attrs)["uidNumber"][0] != "10000" || attrsByName(attrs)["userPassword"][0] != "secret" {
		t.Fatalf("userAddAttributes = %#v, want uid fallback and password", attrs)
	}

	entry := ldap.NewEntry("uid=alice,ou=users,dc=example,dc=org", map[string][]string{
		"cn":        {"Alice Example"},
		"uidNumber": {"10001"},
	})
	snapshot := snapshotLDAPEntry(entry, []string{"cn", "uidNumber", "missing"})
	if snapshot["cn"][0] != "Alice Example" || snapshot["uidNumber"][0] != "10001" {
		t.Fatalf("snapshot = %#v, want selected attrs", snapshot)
	}
	if len(ldapSnapshotAttributes()) == 0 {
		t.Fatal("ldapSnapshotAttributes returned empty list")
	}
}

func TestGoLDAPRestoreRequestDeletesAttributesAbsentFromSnapshot(t *testing.T) {
	result := ldapUpsertResult{
		DN:            "uid=alice,ou=users,dc=example,dc=org",
		Previous:      map[string][]string{"cn": {"Alice Old"}, "gidNumber": {"5002"}},
		ModifiedAttrs: []string{"cn", "mail", "gidNumber"},
	}
	modify := ldapRestoreModifyRequest(result)
	if len(modify.Changes) != 3 {
		t.Fatalf("restore changes = %#v, want replace/delete/replace", modify.Changes)
	}
	if modify.Changes[0].Operation != ldap.ReplaceAttribute || modify.Changes[0].Modification.Type != "cn" || modify.Changes[0].Modification.Vals[0] != "Alice Old" {
		t.Fatalf("cn restore change = %#v", modify.Changes[0])
	}
	if modify.Changes[1].Operation != ldap.DeleteAttribute || modify.Changes[1].Modification.Type != "mail" || len(modify.Changes[1].Modification.Vals) != 0 {
		t.Fatalf("mail restore change = %#v, want delete absent attr", modify.Changes[1])
	}
	if modify.Changes[2].Operation != ldap.ReplaceAttribute || modify.Changes[2].Modification.Type != "gidNumber" || modify.Changes[2].Modification.Vals[0] != "5002" {
		t.Fatalf("gid restore change = %#v", modify.Changes[2])
	}
}

func testGoLDAPDirectory() *goLDAPDirectory {
	return newGoLDAPDirectory(platform.Config{
		LDAPHost:           "ldap.local",
		LDAPPort:           1389,
		LDAPUserSearchBase: "ou=users,dc=example,dc=org",
		LDAPUserFilter:     "(uid=%s)",
		AdapterTimeout:     10 * time.Millisecond,
	})
}

func TestLDAPAttributeHelpers(t *testing.T) {
	user := map[string]any{
		"username":    "alice",
		"full_name":   "Alice Example",
		"email":       "",
		"system_role": 0,
	}
	if ldapCommonName(user) != "Alice Example" || ldapSurname(user) != "Example" {
		t.Fatalf("name helpers = %q/%q", ldapCommonName(user), ldapSurname(user))
	}
	if ldapMail(user) != "alice@platform.local" {
		t.Fatalf("ldapMail = %q, want fallback mail", ldapMail(user))
	}
	if ldapGIDNumber(user) != "5001" {
		t.Fatalf("admin gid = %q, want 5001", ldapGIDNumber(user))
	}
	user["system_role"] = 1
	if ldapGIDNumber(user) != "5003" {
		t.Fatalf("manager gid = %q, want 5003", ldapGIDNumber(user))
	}
	user["system_role"] = 2
	if ldapGIDNumber(user) != "5002" {
		t.Fatalf("user gid = %q, want 5002", ldapGIDNumber(user))
	}
	user = map[string]any{"username": "bob", "email": "bob@example.org", "role": "manager"}
	if ldapSurname(user) != "bob" || ldapMail(user) != "bob@example.org" || ldapGIDNumber(user) != "5003" {
		t.Fatalf("fallback helpers = surname %q mail %q gid %q", ldapSurname(user), ldapMail(user), ldapGIDNumber(user))
	}
}

func TestLDAPSanitizedErrorsAndPayloadProjection(t *testing.T) {
	if sanitizeLDAPError(errLDAPInvalidCredentials) != "ldap invalid credentials" {
		t.Fatalf("invalid credential error sanitized incorrectly")
	}
	if sanitizeLDAPError(errors.New("bind password secret")) != "ldap unavailable" {
		t.Fatalf("unexpected LDAP error leaked detail")
	}
	projected := ldapUserFromPayload(map[string]any{
		"id":       "US1",
		"username": " Alice ",
		"name":     "Alice A",
		"role":     "manager",
		"role_id":  "RO2600003",
		"status":   "online",
	})
	if projected["username"] != "Alice" || projected["full_name"] != "Alice A" || projected["system_role"] != 1 {
		t.Fatalf("projected LDAP user = %#v", projected)
	}
}

func attrsByName(attrs []ldap.Attribute) map[string][]string {
	out := map[string][]string{}
	for _, attr := range attrs {
		out[attr.Type] = attr.Vals
	}
	return out
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func lastIdentityEvent(app *platform.App, name string) *contracts.Event {
	events := app.Events.Outbox()
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Name == name {
			return &events[i]
		}
	}
	return nil
}

type fakeLDAPDirectory struct {
	passwords       map[string]string
	authAttempts    map[string]int
	usernames       map[string]bool
	previousAttrs   map[string]map[string][]string
	upsertErr       map[string]error
	deleteErr       map[string]error
	listErr         error
	upserts         []string
	deletes         []string
	restoredUpserts []ldapUpsertResult
	restoredDeleted []string
}

func newFakeLDAPDirectory() *fakeLDAPDirectory {
	return &fakeLDAPDirectory{
		passwords:     map[string]string{},
		authAttempts:  map[string]int{},
		usernames:     map[string]bool{},
		previousAttrs: map[string]map[string][]string{},
		upsertErr:     map[string]error{},
		deleteErr:     map[string]error{},
	}
}

func (f *fakeLDAPDirectory) Authenticate(_ context.Context, username, password string) (string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	f.authAttempts[username]++
	if f.passwords[username] == password {
		return username, nil
	}
	return "", errLDAPInvalidCredentials
}

func (f *fakeLDAPDirectory) UpsertUser(_ context.Context, user map[string]any, password string, options ...ldapUpsertOption) (ldapUpsertResult, error) {
	username := strings.ToLower(strings.TrimSpace(textValue(user, "username")))
	upsertOptions := ldapUpsertOptionsFrom(options)
	created := !f.usernames[username]
	result := ldapUpsertResult{
		Username:      username,
		DN:            "uid=" + username + ",ou=users,dc=example,dc=org",
		Created:       created,
		Previous:      map[string][]string{"cn": {"previous-" + username}},
		ModifiedAttrs: ldapModifiedAttributes(password, upsertOptions),
	}
	if previous, ok := f.previousAttrs[username]; ok {
		result.Previous = previous
	}
	if err := f.upsertErr[username]; err != nil {
		return result, err
	}
	f.usernames[username] = true
	f.upserts = append(f.upserts, username)
	if strings.TrimSpace(password) != "" && (created || !upsertOptions.preserveExistingPassword) {
		f.passwords[username] = password
	}
	return result, nil
}

func (f *fakeLDAPDirectory) RestoreUpsert(_ context.Context, result ldapUpsertResult) error {
	f.restoredUpserts = append(f.restoredUpserts, result)
	if result.Created {
		delete(f.usernames, strings.ToLower(result.Username))
	}
	return nil
}

func (f *fakeLDAPDirectory) DeleteUser(_ context.Context, user map[string]any) error {
	username := strings.ToLower(strings.TrimSpace(textValue(user, "username")))
	if err := f.deleteErr[username]; err != nil {
		return err
	}
	f.deletes = append(f.deletes, username)
	delete(f.usernames, username)
	return nil
}

func (f *fakeLDAPDirectory) RestoreDeletedUser(_ context.Context, user map[string]any) error {
	username := strings.ToLower(strings.TrimSpace(textValue(user, "username")))
	f.restoredDeleted = append(f.restoredDeleted, username)
	f.usernames[username] = true
	return nil
}

func (f *fakeLDAPDirectory) ListUsernames(context.Context) (map[string]bool, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := map[string]bool{}
	for username, ok := range f.usernames {
		out[username] = ok
	}
	return out, nil
}

type failingIdentityStore struct {
	platform.RecordStore
	failCreateUsers bool
	failUpdateUsers bool
	failDeleteUsers bool
}

func (s *failingIdentityStore) Create(ctx context.Context, resource string, data map[string]any) (contracts.Record[map[string]any], error) {
	if s.failCreateUsers && resource == usersResource {
		return contracts.Record[map[string]any]{}, errors.New("forced create failure")
	}
	return s.RecordStore.Create(ctx, resource, data)
}

func (s *failingIdentityStore) Update(ctx context.Context, resource, id string, data map[string]any) (contracts.Record[map[string]any], bool) {
	if s.failUpdateUsers && resource == usersResource && id != "ADMIN" {
		return contracts.Record[map[string]any]{}, false
	}
	return s.RecordStore.Update(ctx, resource, id, data)
}

func (s *failingIdentityStore) Delete(ctx context.Context, resource, id string) bool {
	if s.failDeleteUsers && resource == usersResource && id != "ADMIN" {
		return false
	}
	return s.RecordStore.Delete(ctx, resource, id)
}

func newLDAPIdentityTestApp(t *testing.T, fake *fakeLDAPDirectory) *platform.App {
	t.Helper()
	withFakeLDAPDirectory(t, fake)
	app := platform.NewApp(ldapTestConfig())
	Register(app)
	return app
}

func withFakeLDAPDirectory(t *testing.T, fake *fakeLDAPDirectory) {
	t.Helper()
	previous := newLDAPDirectory
	newLDAPDirectory = func(platform.Config) ldapDirectory { return fake }
	t.Cleanup(func() { newLDAPDirectory = previous })
}

func ldapTestConfig() platform.Config {
	return platform.Config{
		ServiceName:            serviceName,
		HTTPAddr:               ":0",
		RequireAuth:            true,
		LDAPEnabled:            true,
		LDAPHost:               "ldap.local",
		LDAPPort:               1389,
		LDAPBindDN:             "cn=admin,dc=example,dc=org",
		LDAPBindPassword:       "secret",
		LDAPUserSearchBase:     "ou=users,dc=example,dc=org",
		LDAPUserFilter:         "(uid=%s)",
		LDAPMirrorSyncInterval: 5 * time.Minute,
	}
}

func seedLDAPIdentityUser(t *testing.T, app *platform.App, id, username, password, status string) {
	t.Helper()
	seedStoreIdentityUser(t, app.Store, id, username, password, status)
}

func seedStoreIdentityUser(t *testing.T, store platform.RecordStore, id, username, password, status string) {
	t.Helper()
	if _, err := store.Create(context.Background(), usersResource, map[string]any{
		"id":            id,
		"username":      username,
		"password_hash": platform.HashSecret(password),
		"status":        status,
		"role":          "user",
		"role_id":       defaultRoleID,
		"system_role":   2,
	}); err != nil {
		t.Fatal(err)
	}
}
