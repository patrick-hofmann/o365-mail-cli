package profile

import (
	"fmt"

	"github.com/spf13/cobra"
)

// AnnotationKey is the cobra command annotation key for required permissions.
const AnnotationKey = "required_permission"

// PermissionDeniedError is returned when a command is blocked by a profile.
type PermissionDeniedError struct {
	Command    string
	Permission string
	Profile    string
}

func (e *PermissionDeniedError) Error() string {
	return fmt.Sprintf("permission denied: '%s' requires '%s' (profile: %s)", e.Command, e.Permission, e.Profile)
}

// CheckCommand checks whether the active profile allows the given command.
// A nil profile allows everything.
func CheckCommand(p *Profile, cmd *cobra.Command) error {
	if p == nil {
		return nil
	}

	permission, ok := cmd.Annotations[AnnotationKey]
	if !ok {
		return nil
	}

	if p.IsAllowed(permission) {
		return nil
	}

	return &PermissionDeniedError{
		Command:    fullCommandName(cmd),
		Permission: permission,
		Profile:    p.Name,
	}
}

func fullCommandName(cmd *cobra.Command) string {
	if cmd.Parent() != nil && cmd.Parent().HasParent() {
		return fullCommandName(cmd.Parent()) + " " + cmd.Name()
	}
	return cmd.Name()
}
