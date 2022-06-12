package parseargs

type InvokeArgs struct {
	Action string
	IsDry bool
	Files []string
}

func ParseArgs(args []string) InvokeArgs {
	ia := &InvokeArgs{}

	state := ""

	firstArg := args[0]

	if firstArg == "sync" {
		ia.Action = firstArg

	} else if firstArg == "--help" || firstArg == "-h" || firstArg == "help" {
		ia.Action = "help"
		return *ia

	}

	for _, arg := range args[1:] {
		if state == "file" {
			state = ""
			if ia.Files == nil {
				ia.Files = []string{}
			}
			ia.Files = append(ia.Files, arg)

		} else if arg == "--dry" {
			ia.IsDry = true

		} else if arg == "--file" {
			state = "file"

		}
	}

	if ia.Files == nil {
		ia.Files = []string{"secrets.yml"}
	}

	return *ia
}
