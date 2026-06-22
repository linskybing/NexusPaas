package platform

// typedPostgresResourceFor resolves a resource key to its owned, typed-table
// spec for any service that has migrated off the generic platform_records store.
// Centralizing the lookup keeps PostgresStore's methods Open/Closed: a new typed
// domain registers its specs in a store_postgres_<service>.go file and is picked
// up here without editing every store method. The identityPostgresResource struct
// is the shared typed-table descriptor (named for its first adopter, identity).
func typedPostgresResourceFor(resource string) (identityPostgresResource, bool) {
	if spec, ok := identityPostgresResourceFor(resource); ok {
		return spec, true
	}
	return requestNotificationPostgresResourceFor(resource)
}
