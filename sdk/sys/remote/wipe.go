package remote

import (
	"context"
	"errors"
	"fmt"
	"os"
)

func wipeDest(_ context.Context, dest string) error {
	if err := canWipe(dest); err != nil {
		return err
	}
	if err := os.RemoveAll(dest); err != nil {

		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("remove %s: %w", dest, err)
	}

	forgetDest(dest)
	return nil
}
