package platform

import "time"

const imageRegistryBuildJobsResource = "image-registry-service:image_build_jobs"

// imageRegistryPostgresResources maps image-registry build-job records to their
// typed, service-owned table (migration 0002). The full record stays in the
// payload JSONB column; the promoted columns give indexed project/status queries
// and clear ownership of production-critical supply-chain state. The
// identityPostgresResource struct is the shared typed-table descriptor (named for
// its first adopter); see typedPostgresResourceFor. (P0-5)
var imageRegistryPostgresResources = map[string]identityPostgresResource{
	imageRegistryBuildJobsResource: {
		resource: imageRegistryBuildJobsResource,
		table:    "image_build_jobs",
		insert:   imageRegistryBuildInsertColumns,
		update:   imageRegistryBuildUpdateColumns,
	},
}

func imageRegistryPostgresResourceFor(resource string) (identityPostgresResource, bool) {
	spec, ok := imageRegistryPostgresResources[resource]
	return spec, ok
}

func imageRegistryBuildInsertColumns(data map[string]any, _ string, _ time.Time) []identityColumnValue {
	return []identityColumnValue{
		{"project_id", identityTextDefault(data, "", "project_id", "projectId")},
		{"image_reference", identityTextDefault(data, "", "image_reference", "imageReference")},
		{"build_type", identityTextDefault(data, "", "build_type", "buildType")},
		{"status", identityTextDefault(data, "queued", "status")},
		{"requested_by", identityTextDefault(data, "", "requested_by", "requestedBy")},
		{"source_digest", identityNullableText(data, "source_digest", "sourceDigest")},
	}
}

func imageRegistryBuildUpdateColumns(data map[string]any) []identityColumnValue {
	return identityColumnsFromData(data, []identityColumnReader{
		{"project_id", identityTextUpdate("project_id", "projectId")},
		{"image_reference", identityTextUpdate("image_reference", "imageReference")},
		{"build_type", identityTextUpdate("build_type", "buildType")},
		{"status", identityTextUpdate("status")},
		{"requested_by", identityTextUpdate("requested_by", "requestedBy")},
		{"source_digest", identityNullableTextUpdate("source_digest", "sourceDigest")},
	})
}
