//go:build integration

package executor

import "context"

func createDirectoryWithPermissions(ctx context.Context, path, mode, owner, group string, recursive bool) error {
	return testExecutor().createDirectoryWithPermissions(ctx, path, mode, owner, group, recursive)
}

func removeDirectory(ctx context.Context, path string) error {
	return testExecutor().removeDirectory(ctx, path)
}

func userExists(ctx context.Context, username string) (bool, error) {
	return testExecutor().userExists(ctx, username)
}

func groupExists(ctx context.Context, groupName string) (bool, error) {
	return testExecutor().groupExists(ctx, groupName)
}

func userInGroup(ctx context.Context, username, groupName string) bool {
	return testExecutor().userInGroup(ctx, username, groupName)
}
