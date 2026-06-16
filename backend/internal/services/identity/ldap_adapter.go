package identity

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/linskybing/nexuspaas/backend/internal/platform"
	"github.com/linskybing/nexuspaas/backend/internal/services/shared"

	"github.com/go-ldap/ldap/v3"
)

type goLDAPDirectory struct {
	cfg platform.Config
}

func newGoLDAPDirectory(cfg platform.Config) *goLDAPDirectory {
	return &goLDAPDirectory{cfg: cfg}
}

func (d *goLDAPDirectory) Authenticate(ctx context.Context, username, password string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return "", errLDAPInvalidCredentials
	}
	conn, err := d.adminConn(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()
	entry, err := d.findUser(ctx, conn, username, []string{"uid"})
	if err != nil {
		return "", errLDAPInvalidCredentials
	}
	userConn, err := d.dial(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = userConn.Close() }()
	if err := userConn.Bind(entry.DN, password); err != nil {
		return "", errLDAPInvalidCredentials
	}
	return shared.FirstNonEmpty(entry.GetAttributeValue("uid"), username), nil
}

func (d *goLDAPDirectory) UpsertUser(ctx context.Context, user map[string]any, password string, options ...ldapUpsertOption) (ldapUpsertResult, error) {
	username := strings.TrimSpace(textValue(user, "username"))
	if username == "" {
		return ldapUpsertResult{}, errLDAPNotConfigured
	}
	upsertOptions := ldapUpsertOptionsFrom(options)
	conn, err := d.adminConn(ctx)
	if err != nil {
		return ldapUpsertResult{}, err
	}
	defer func() { _ = conn.Close() }()

	dn := d.userDN(username)
	result := ldapUpsertResult{Username: username, DN: dn}
	attrs := ldapSnapshotAttributes()
	entry, err := d.findUser(ctx, conn, username, attrs)
	if err == nil && entry != nil {
		result.Previous = snapshotLDAPEntry(entry, attrs)
		result.ModifiedAttrs = ldapModifiedAttributes(password, upsertOptions)
		return result, d.modifyUser(conn, dn, user, ldapModifyPassword(password, upsertOptions))
	}
	add := ldap.NewAddRequest(dn, nil)
	for _, attr := range d.userAddAttributes(ctx, conn, user, password) {
		add.Attribute(attr.Type, attr.Vals)
	}
	if err := conn.Add(add); err != nil {
		if !ldap.IsErrorWithCode(err, ldap.LDAPResultEntryAlreadyExists) {
			return ldapUpsertResult{}, err
		}
		entry, findErr := d.findUser(ctx, conn, username, attrs)
		if findErr == nil && entry != nil {
			result.Previous = snapshotLDAPEntry(entry, attrs)
		}
		result.ModifiedAttrs = ldapModifiedAttributes(password, upsertOptions)
		return result, d.modifyUser(conn, dn, user, ldapModifyPassword(password, upsertOptions))
	}
	result.Created = true
	return result, nil
}

func (d *goLDAPDirectory) RestoreUpsert(ctx context.Context, result ldapUpsertResult) error {
	if strings.TrimSpace(result.Username) == "" {
		return nil
	}
	conn, err := d.adminConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if result.Created {
		return normalizeLDAPDeleteError(conn.Del(ldap.NewDelRequest(result.DN, nil)))
	}
	if len(result.Previous) == 0 {
		return nil
	}
	modify := ldapRestoreModifyRequest(result)
	if len(modify.Changes) == 0 {
		return nil
	}
	return conn.Modify(modify)
}

func (d *goLDAPDirectory) DeleteUser(ctx context.Context, user map[string]any) error {
	username := strings.TrimSpace(textValue(user, "username"))
	if username == "" {
		return nil
	}
	conn, err := d.adminConn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	return normalizeLDAPDeleteError(conn.Del(ldap.NewDelRequest(d.userDN(username), nil)))
}

func (d *goLDAPDirectory) RestoreDeletedUser(ctx context.Context, user map[string]any) error {
	_, err := d.UpsertUser(ctx, user, "")
	return err
}

func (d *goLDAPDirectory) ListUsernames(ctx context.Context) (map[string]bool, error) {
	conn, err := d.adminConn(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	search := ldap.NewSearchRequest(
		d.cfg.LDAPUserSearchBase,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		d.searchTimeLimit(),
		false,
		"(uid=*)",
		[]string{"uid"},
		nil,
	)
	result, err := conn.Search(search)
	if err != nil {
		return nil, err
	}
	usernames := map[string]bool{}
	for _, entry := range result.Entries {
		if username := strings.TrimSpace(entry.GetAttributeValue("uid")); username != "" {
			usernames[strings.ToLower(username)] = true
		}
	}
	return usernames, nil
}

func (d *goLDAPDirectory) adminConn(ctx context.Context) (*ldap.Conn, error) {
	conn, err := d.dial(ctx)
	if err != nil {
		return nil, err
	}
	if err := conn.Bind(d.cfg.LDAPBindDN, d.cfg.LDAPBindPassword); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func (d *goLDAPDirectory) dial(ctx context.Context) (*ldap.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	scheme := "ldap"
	if d.cfg.LDAPUseTLS {
		scheme = "ldaps"
	}
	url := fmt.Sprintf("%s://%s:%d", scheme, d.cfg.LDAPHost, d.cfg.LDAPPort)
	timeout := d.timeout()
	opts := []ldap.DialOpt{ldap.DialWithDialer(&net.Dialer{Timeout: timeout})}
	if d.cfg.LDAPUseTLS {
		opts = append(opts, ldap.DialWithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12}))
	}
	conn, err := ldap.DialURL(url, opts...)
	if err != nil {
		return nil, err
	}
	conn.SetTimeout(timeout)
	return conn, nil
}

func (d *goLDAPDirectory) findUser(ctx context.Context, conn *ldap.Conn, username string, attrs []string) (*ldap.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	filter, err := d.userFilter(username)
	if err != nil {
		return nil, err
	}
	search := ldap.NewSearchRequest(
		d.cfg.LDAPUserSearchBase,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		2,
		d.searchTimeLimit(),
		false,
		filter,
		attrs,
		nil,
	)
	result, err := conn.Search(search)
	if err != nil {
		return nil, err
	}
	if len(result.Entries) != 1 {
		return nil, errLDAPInvalidCredentials
	}
	return result.Entries[0], nil
}

func (d *goLDAPDirectory) userFilter(username string) (string, error) {
	if strings.Count(d.cfg.LDAPUserFilter, "%s") != 1 {
		return "", errLDAPNotConfigured
	}
	return fmt.Sprintf(d.cfg.LDAPUserFilter, ldap.EscapeFilter(strings.TrimSpace(username))), nil
}

func (d *goLDAPDirectory) userDN(username string) string {
	return fmt.Sprintf("uid=%s,%s", ldap.EscapeDN(strings.TrimSpace(username)), d.cfg.LDAPUserSearchBase)
}

func (d *goLDAPDirectory) userAddAttributes(ctx context.Context, conn *ldap.Conn, user map[string]any, password string) []ldap.Attribute {
	username := strings.TrimSpace(textValue(user, "username"))
	attrs := []ldap.Attribute{
		{Type: "objectClass", Vals: []string{"inetOrgPerson", "posixAccount"}},
		{Type: "cn", Vals: []string{ldapCommonName(user)}},
		{Type: "sn", Vals: []string{ldapSurname(user)}},
		{Type: "uid", Vals: []string{username}},
		{Type: "uidNumber", Vals: []string{d.nextUIDNumber(ctx, conn)}},
		{Type: "gidNumber", Vals: []string{ldapGIDNumber(user)}},
		{Type: "homeDirectory", Vals: []string{"/home/" + username}},
		{Type: "mail", Vals: []string{ldapMail(user)}},
	}
	if strings.TrimSpace(password) != "" {
		attrs = append(attrs, ldap.Attribute{Type: "userPassword", Vals: []string{password}})
	}
	return attrs
}

func (d *goLDAPDirectory) modifyUser(conn *ldap.Conn, dn string, user map[string]any, password string) error {
	modify := ldap.NewModifyRequest(dn, nil)
	modify.Replace("objectClass", []string{"inetOrgPerson", "posixAccount"})
	modify.Replace("cn", []string{ldapCommonName(user)})
	modify.Replace("sn", []string{ldapSurname(user)})
	modify.Replace("mail", []string{ldapMail(user)})
	modify.Replace("gidNumber", []string{ldapGIDNumber(user)})
	if strings.TrimSpace(password) != "" {
		modify.Replace("userPassword", []string{password})
	}
	return conn.Modify(modify)
}

func (d *goLDAPDirectory) nextUIDNumber(ctx context.Context, conn *ldap.Conn) string {
	if err := ctx.Err(); err != nil {
		return "10000"
	}
	search := ldap.NewSearchRequest(
		d.cfg.LDAPUserSearchBase,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		d.searchTimeLimit(),
		false,
		"(uid=*)",
		[]string{"uidNumber"},
		nil,
	)
	result, err := conn.Search(search)
	if err != nil {
		return "10000"
	}
	maxUID := 9999
	for _, entry := range result.Entries {
		if n, err := strconv.Atoi(entry.GetAttributeValue("uidNumber")); err == nil && n > maxUID {
			maxUID = n
		}
	}
	return strconv.Itoa(maxUID + 1)
}

func (d *goLDAPDirectory) timeout() time.Duration {
	if d.cfg.AdapterTimeout > 0 {
		return d.cfg.AdapterTimeout
	}
	return 2 * time.Second
}

func (d *goLDAPDirectory) searchTimeLimit() int {
	seconds := int(d.timeout().Seconds())
	if seconds < 1 {
		return 1
	}
	return seconds
}

func ldapSnapshotAttributes() []string {
	return []string{"objectClass", "cn", "sn", "uid", "uidNumber", "gidNumber", "homeDirectory", "mail"}
}

func ldapUpsertOptionsFrom(optionFns []ldapUpsertOption) ldapUpsertOptions {
	var options ldapUpsertOptions
	for _, optionFn := range optionFns {
		if optionFn != nil {
			optionFn(&options)
		}
	}
	return options
}

func ldapModifyPassword(password string, options ldapUpsertOptions) string {
	if options.preserveExistingPassword {
		return ""
	}
	return password
}

func ldapModifiedAttributes(password string, options ldapUpsertOptions) []string {
	attrs := []string{"objectClass", "cn", "sn", "mail", "gidNumber"}
	if strings.TrimSpace(password) != "" && !options.preserveExistingPassword {
		attrs = append(attrs, "userPassword")
	}
	return attrs
}

func ldapRestoreModifyRequest(result ldapUpsertResult) *ldap.ModifyRequest {
	modify := ldap.NewModifyRequest(result.DN, nil)
	attrs := result.ModifiedAttrs
	if len(attrs) == 0 {
		attrs = ldapSnapshotAttributes()
	}
	for _, attr := range attrs {
		if values, ok := result.Previous[attr]; ok {
			modify.Replace(attr, values)
			continue
		}
		modify.Delete(attr, nil)
	}
	return modify
}

func snapshotLDAPEntry(entry *ldap.Entry, attrs []string) map[string][]string {
	out := map[string][]string{}
	for _, attr := range attrs {
		values := entry.GetAttributeValues(attr)
		if values == nil {
			continue
		}
		out[attr] = append([]string{}, values...)
	}
	return out
}

func normalizeLDAPDeleteError(err error) error {
	if err != nil && ldap.IsErrorWithCode(err, ldap.LDAPResultNoSuchObject) {
		return nil
	}
	return err
}

func ldapCommonName(user map[string]any) string {
	return shared.FirstNonEmpty(strings.TrimSpace(textValue(user, "full_name")), strings.TrimSpace(textValue(user, "name")), strings.TrimSpace(textValue(user, "username")))
}

func ldapSurname(user map[string]any) string {
	name := strings.TrimSpace(ldapCommonName(user))
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return strings.TrimSpace(textValue(user, "username"))
	}
	return parts[len(parts)-1]
}

func ldapMail(user map[string]any) string {
	username := strings.TrimSpace(textValue(user, "username"))
	return shared.FirstNonEmpty(strings.TrimSpace(textValue(user, "email")), username+"@platform.local")
}

func ldapGIDNumber(user map[string]any) string {
	switch intValue(user, "system_role", systemRoleFor(textValue(user, "role"), 2)) {
	case 0:
		return "5001"
	case 1:
		return "5003"
	default:
		return "5002"
	}
}
