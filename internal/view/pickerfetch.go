// pickerfetch.go shows that a diff is on its way: a spinner in the picker's notice
// row while a slow fetch runs.

package view

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// fetchRevealMsg arrives once a fetch has run for spinnerDelay, the point at which
// it is worth saying that the diff is on its way.
type fetchRevealMsg struct {
	fetch int
}

// fetchFrameMsg advances a running fetch's spinner.
type fetchFrameMsg struct {
	fetch int
}

// showFetchProgress raises the spinner for fetch if that fetch is still running.
func (m pickerModel) showFetchProgress(fetch int) (tea.Model, tea.Cmd) {
	if !m.fetching || fetch != m.fetchID {
		return m, nil
	}
	m.fetchFrame = 0
	return m.drawFetchProgress()
}

// advanceFetchProgress turns the spinner for fetch if that fetch is still running.
func (m pickerModel) advanceFetchProgress(fetch int) (tea.Model, tea.Cmd) {
	if !m.fetching || fetch != m.fetchID {
		return m, nil
	}
	m.fetchFrame = (m.fetchFrame + 1) % len(spinnerFrames)
	return m.drawFetchProgress()
}

// drawFetchProgress shows the spinner's current frame in the notice row and schedules
// the next one. It uses the comparison spinner's frames and pace so both waits look
// the same.
func (m pickerModel) drawFetchProgress() (tea.Model, tea.Cmd) {
	m.notice = spinnerFrames[m.fetchFrame] + " fetching diff…"
	m.offset = m.scrollOffset()
	id := m.fetchID
	return m, tea.Tick(spinnerInterval, func(time.Time) tea.Msg { return fetchFrameMsg{fetch: id} })
}
