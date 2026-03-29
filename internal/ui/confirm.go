package ui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ConfirmDialog creates a centered modal confirmation dialog.
// Cancel is first (default focus) — safer for destructive action.
func ConfirmDialog(sessionName string, onConfirm, onCancel func()) *tview.Modal {
	modal := tview.NewModal()
	modal.SetText(fmt.Sprintf("Delete session \"%s\"?", sessionName))
	modal.AddButtons([]string{"Cancel", "Confirm"})
	modal.SetBackgroundColor(BgMain)
	// Unfocused buttons: dim background
	modal.SetButtonBackgroundColor(BgBorder)
	modal.SetButtonTextColor(TextDim)
	// Focused button: bright orange on dark — clearly distinguishable
	modal.SetButtonActivatedStyle(tcell.StyleDefault.
		Foreground(BgMain).Background(ClaudeOrange).Bold(true))

	modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		if buttonLabel == "Confirm" {
			onConfirm()
		} else {
			onCancel()
		}
	})

	return modal
}

// ServerDialog creates a form for adding a remote server.
func ServerDialog(onSubmit func(name, host string, port int, keyFile string), onCancel func()) *tview.Form {
	form := tview.NewForm()
	form.SetBackgroundColor(BgMain)
	form.SetBorder(true)
	form.SetBorderColor(ClaudeOrange)
	form.SetTitle(" Add Remote Server ")
	form.SetTitleColor(ClaudeOrange)

	form.AddInputField("Name", "", 20, nil, nil)
	form.AddInputField("Host (user@host)", "", 30, nil, nil)
	form.AddInputField("Port", "22", 6, nil, nil)
	form.AddInputField("Key File", "~/.ssh/id_rsa", 30, nil, nil)

	form.AddButton("Add", func() {
		name := form.GetFormItemByLabel("Name").(*tview.InputField).GetText()
		host := form.GetFormItemByLabel("Host (user@host)").(*tview.InputField).GetText()
		portStr := form.GetFormItemByLabel("Port").(*tview.InputField).GetText()
		keyFile := form.GetFormItemByLabel("Key File").(*tview.InputField).GetText()

		port := 22
		fmt.Sscanf(portStr, "%d", &port)

		onSubmit(name, host, port, keyFile)
	})

	form.AddButton("Cancel", func() {
		onCancel()
	})

	return form
}
