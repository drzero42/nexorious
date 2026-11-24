package clientops

import (
	"context"
	"fmt"

	"github.com/drzero42/nexorious/internal/cliclient"
)

// smellIDsPerRequest is the most user_game_ids one apply/ignore/restore request
// may carry. It mirrors the server's cap (maxSmellIDsPerRequest in
// internal/api/library_smells.go), which answers a longer list with a 400.
const smellIDsPerRequest = 200

// CollectFlaggedIDs returns every flagged user_game_id for a check, across pages.
func CollectFlaggedIDs(ctx context.Context, c *cliclient.Client, key, checkID string) ([]string, error) {
	var ids []string
	for page := 1; ; page++ {
		res, err := c.ListSmellItems(ctx, key, checkID, page, smellIDsPerRequest)
		if err != nil {
			return nil, err
		}
		for i := range res.Items {
			ids = append(ids, res.Items[i].UserGameID)
		}
		if res.Pages <= page {
			break
		}
	}
	return ids, nil
}

// ApplySmell applies a check's auto-fix to ids, in requests of at most
// smellIDsPerRequest, and sums the applied/skipped counts. The batches are not
// atomic together: see partialSmellErr for a failure part way through.
func ApplySmell(ctx context.Context, c *cliclient.Client, key, checkID string, ids []string) (*cliclient.SmellApplyResult, error) {
	var total cliclient.SmellApplyResult
	sent, err := eachSmellIDBatch(ids, func(batch []string) error {
		res, err := c.ApplySmell(ctx, key, checkID, batch)
		if err != nil {
			return err
		}
		total.Applied += res.Applied
		total.Skipped += res.Skipped
		return nil
	})
	if err != nil {
		return nil, partialSmellErr(sent, len(ids), fmt.Sprintf("applied %d, skipped %d", total.Applied, total.Skipped), err)
	}
	return &total, nil
}

// IgnoreSmell dismisses ids for a check in batches; returns the count newly ignored.
func IgnoreSmell(ctx context.Context, c *cliclient.Client, key, checkID string, ids []string) (int, error) {
	var ignored int
	sent, err := eachSmellIDBatch(ids, func(batch []string) error {
		n, err := c.IgnoreSmell(ctx, key, checkID, batch)
		ignored += n
		return err
	})
	if err != nil {
		return 0, partialSmellErr(sent, len(ids), fmt.Sprintf("dismissed %d", ignored), err)
	}
	return ignored, nil
}

// RestoreSmell un-dismisses ids for a check in batches; returns the count restored.
func RestoreSmell(ctx context.Context, c *cliclient.Client, key, checkID string, ids []string) (int, error) {
	var restored int
	sent, err := eachSmellIDBatch(ids, func(batch []string) error {
		n, err := c.RestoreSmell(ctx, key, checkID, batch)
		restored += n
		return err
	})
	if err != nil {
		return 0, partialSmellErr(sent, len(ids), fmt.Sprintf("restored %d", restored), err)
	}
	return restored, nil
}

// eachSmellIDBatch calls fn on consecutive batches of ids, stopping at the
// first error. sent is how many ids were in the batches that succeeded.
func eachSmellIDBatch(ids []string, fn func(batch []string) error) (sent int, err error) {
	for start := 0; start < len(ids); start += smellIDsPerRequest {
		end := min(start+smellIDsPerRequest, len(ids))
		if err := fn(ids[start:end]); err != nil {
			return start, err
		}
	}
	return len(ids), nil
}

// partialSmellErr reports a batch failure. When nothing was sent yet the
// server's error is returned unwrapped, so callers that rewrite a bare
// *cliclient.HTTPError (mcptools.toolError: 404 unknown check, 422 not
// auto-fixable) still do. Once a batch has gone through, the error says how far
// it got; every operation is safe to re-run (apply re-validates what is still
// flagged, ignore is idempotent, restore of a restored game is a no-op).
func partialSmellErr(sent, total int, progress string, err error) error {
	if sent == 0 {
		return err
	}
	return fmt.Errorf("stopped after %d of %d game(s) (%s); re-run to finish: %w", sent, total, progress, err)
}
