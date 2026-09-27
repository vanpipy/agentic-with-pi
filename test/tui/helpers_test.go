package tui_test

import (
	"time"

	"charm.land/bubbles/v2/spinner"
)

func spinnerTickMsg() spinner.TickMsg {
	return spinner.TickMsg{Time: time.Unix(0, 0), ID: 0}
}
