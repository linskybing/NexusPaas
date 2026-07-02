package platform

import "time"

const (
	orgProjectProjectsResource = "org-project-service:projects"
	orgProjectMembersResource  = "org-project-service:project_members"
)

// orgProjectPostgresResources maps org-project project + membership records to
// their typed, service-owned tables (migration 0002). Projects and members are
// the authz-critical aggregates every other service reads; the full record
// stays in the payload JSONB column so reads reconstruct identical maps. The
// identityPostgresResource struct is the shared typed-table descriptor (named
// for its first adopter); see typedPostgresResourceFor.
var orgProjectPostgresResources = map[string]identityPostgresResource{
	orgProjectProjectsResource: {
		resource: orgProjectProjectsResource,
		table:    "org_projects",
		insert:   orgProjectInsertColumns,
		update:   orgProjectUpdateColumns,
	},
	orgProjectMembersResource: {
		resource: orgProjectMembersResource,
		table:    "org_project_members",
		insert:   orgProjectMemberInsertColumns,
		update:   orgProjectMemberUpdateColumns,
	},
}

func orgProjectPostgresResourceFor(resource string) (identityPostgresResource, bool) {
	spec, ok := orgProjectPostgresResources[resource]
	return spec, ok
}

func orgProjectInsertColumns(data map[string]any, _ string, _ time.Time) []identityColumnValue {
	return []identityColumnValue{
		{"project_name", identityTextDefault(data, "", "project_name", "ProjectName", "name")},
		{"owner_id", identityTextDefault(data, "", "owner_id", "g_id", "GID")},
		{"created_by", identityTextDefault(data, "", "created_by", "createdBy")},
	}
}

func orgProjectUpdateColumns(data map[string]any) []identityColumnValue {
	return identityColumnsFromData(data, []identityColumnReader{
		{"project_name", identityTextUpdate("project_name", "ProjectName", "name")},
		{"owner_id", identityTextUpdate("owner_id", "g_id", "GID")},
		{"created_by", identityTextUpdate("created_by", "createdBy")},
	})
}

func orgProjectMemberInsertColumns(data map[string]any, _ string, _ time.Time) []identityColumnValue {
	return []identityColumnValue{
		{"project_id", identityTextDefault(data, "", "project_id", "projectId")},
		{"user_id", identityTextDefault(data, "", "user_id", "userId")},
		{"role", identityTextDefault(data, "user", "role")},
	}
}

func orgProjectMemberUpdateColumns(data map[string]any) []identityColumnValue {
	return identityColumnsFromData(data, []identityColumnReader{
		{"project_id", identityTextUpdate("project_id", "projectId")},
		{"user_id", identityTextUpdate("user_id", "userId")},
		{"role", identityTextUpdate("role")},
	})
}
