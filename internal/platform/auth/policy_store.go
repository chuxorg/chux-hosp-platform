package auth

import "context"

// propertyPermissions loads only permissions backed by a grant in the same
// group as the user's role. Each request gets a fresh database snapshot.
func (s *Store) propertyPermissions(ctx context.Context, userID, propertyID int64) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
SELECT DISTINCT permission.permission
FROM platform_group_role_assignments AS assignment
JOIN platform_users AS identity ON identity.id = assignment.user_id
JOIN platform_role_permissions AS permission ON permission.role_id = assignment.role_id
JOIN platform_group_resource_grants AS resource_grant
    ON resource_grant.group_id = assignment.group_id
   AND resource_grant.resource_type = 'property'
   AND resource_grant.resource_id = $2
WHERE assignment.user_id = $1 AND identity.disabled_at IS NULL
`, userID, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []string
	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return permissions, nil
}
