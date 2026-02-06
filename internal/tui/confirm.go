package tui

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/harness/harness-cli/internal/style"
)

// ConfirmDeletion shows an interactive confirmation prompt for destructive
// operations. Returns true only if the user explicitly confirms.
func ConfirmDeletion(resourceType, resourceName string) (bool, error) {
	var confirmed bool

	header := style.Warning.Render(fmt.Sprintf(
		"⚠  You are about to delete %s %s",
		resourceType,
		style.Bold.Render(resourceName),
	))
	fmt.Println(header)
	fmt.Println()

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Delete %s \"%s\"?", resourceType, resourceName)).
				Description("This action cannot be undone.").
				Affirmative("Yes, delete").
				Negative("No, cancel").
				Value(&confirmed),
		),
	)

	err := form.Run()
	if err != nil {
		return false, err
	}

	return confirmed, nil
}

// PromptInput shows an interactive text input prompt and returns the value.
func PromptInput(title, description, placeholder string) (string, error) {
	var value string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(title).
				Description(description).
				Placeholder(placeholder).
				Value(&value),
		),
	)

	err := form.Run()
	if err != nil {
		return "", err
	}

	return value, nil
}

// PromptSelect shows an interactive selection prompt and returns the chosen value.
func PromptSelect(title, description string, options []string) (string, error) {
	var value string

	opts := make([]huh.Option[string], len(options))
	for i, o := range options {
		opts[i] = huh.NewOption(o, o)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Description(description).
				Options(opts...).
				Value(&value),
		),
	)

	err := form.Run()
	if err != nil {
		return "", err
	}

	return value, nil
}
