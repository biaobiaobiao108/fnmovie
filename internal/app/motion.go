package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const navigationMotionDuration = 160 * time.Millisecond

// Animate one keyed content container, never each item in a virtualized grid.
// MyGo's transitions respect ReduceMotion and schedule frames at display rate.
func pageTransition() ui.ElementTransition {
	return ui.ElementTransition{Duration: navigationMotionDuration, Position: true, Enter: &ui.Motion{Y: 6}}
}

func (a *appState) navigationKey() string {
	if a.selectedPerson != nil {
		return "person:" + a.selectedPerson.ID
	}
	if a.selected != nil {
		return "detail:" + a.selected.ID
	}
	if strings.TrimSpace(a.query) != "" {
		return "search"
	}
	if a.section == "home" && a.home.AllContinue {
		return "home:continue"
	}
	return "section:" + a.section + ":" + a.libraryID
}

func actionButton(c *ui.Context, label string) ui.Element {
	return ui.Button(c, label).Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
}

func primaryActionButton(c *ui.Context, label string) ui.Element {
	return ui.PrimaryButton(c, label).Transition(ui.ElementTransition{Colors: true, Duration: 120 * time.Millisecond})
}
