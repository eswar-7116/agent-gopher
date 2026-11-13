package tui

type Theme struct {
	Name string

	User        string // user label color
	Agent       string // agent label color
	Tool        string // tool call color
	Error       string // error color
	Dim         string // dim text
	Title       string // title bar color
	Border      string // input box border
	Prompt      string // permission prompt border
	InputBorder string // inactive input border
}

var builtinThemes = map[string]Theme{
	"default": {
		Name:        "default",
		User:        "#5fffaf",
		Agent:       "#ff5faf",
		Tool:        "#ffaf00",
		Error:       "#ff0000",
		Dim:         "#626262",
		Title:       "#005fff",
		Border:      "#ffaf00",
		Prompt:      "#ffaf00",
		InputBorder: "#585858",
	},
	"catppuccin": {
		Name:        "catppuccin",
		User:        "#a6e3a1",
		Agent:       "#cba6f7",
		Tool:        "#fab387",
		Error:       "#f38ba8",
		Dim:         "#6c7086",
		Title:       "#89b4fa",
		Border:      "#f9e2af",
		Prompt:      "#fab387",
		InputBorder: "#313244",
	},
	"ayu-dark": {
		Name:        "ayu-dark",
		User:        "#c2d94c",
		Agent:       "#d2a6ff",
		Tool:        "#ff8f40",
		Error:       "#ff3333",
		Dim:         "#626a73",
		Title:       "#59c2ff",
		Border:      "#e6b450",
		Prompt:      "#ff8f40",
		InputBorder: "#0d1017",
	},
}
