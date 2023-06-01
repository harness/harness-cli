package main

func PromptEnvDetails() bool {
	promptConfirm := false

	if len(cliCdReq.Account) == 0 {
		promptConfirm = true
		cliCdReq.Account = TextInput("Account that you wish to login to:")
	}

	return promptConfirm
}

func PromptConnectorDetails() bool {

	return false
}
