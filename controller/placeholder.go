package controller

import (
	"strconv"

	"meth-enginev2/config"
)

func (a *App) placeholder() string {
	return strconv.Itoa(a.Cfg.MaxChars()) + " character limit"
}

func tagsHint(l config.TagLimits) string {
	if l.Max == 1 {
		return "1 tag"
	}
	return "Up to " + strconv.Itoa(l.Max) + " tags, separated by spaces"
}
