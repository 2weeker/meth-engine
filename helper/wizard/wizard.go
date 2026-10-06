package wizard

import (
	"context"
	"log/slog"

	"meth-enginev2/config"
	"meth-enginev2/helper/auth"
	"meth-enginev2/model"
)

func Seed(ctx context.Context, cfg config.Config, store *model.Store) error {
	if err := seedModerator(ctx, cfg, store); err != nil {
		return err
	}

	n, err := store.PostCount(ctx)
	if err != nil {
		return err
	}
	if limit := cfg.Site.PostCap(); n > limit {
		slog.Warn("the site holds more posts than max_posts keeps; the next post deletes the oldest down to the cap",
			"posts", n, "max_posts", limit)
	}
	return nil
}

func seedModerator(ctx context.Context, cfg config.Config, store *model.Store) error {
	n, err := store.UserCount(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if cfg.ModUsername == "" || cfg.ModPassword == "" {
		slog.Warn("no moderators exist and METH_MOD_USERNAME/METH_MOD_PASSWORD are unset; /mod cannot be used")
		return nil
	}
	hash, err := auth.HashPassword(cfg.ModPassword)
	if err != nil {
		return err
	}
	if err := store.CreateUser(ctx, cfg.ModUsername, hash); err != nil {
		return err
	}
	slog.Info("created moderator", "username", cfg.ModUsername)
	return nil
}
