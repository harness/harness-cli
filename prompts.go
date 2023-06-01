package main

func PromptAccountDetails() bool {
	promptConfirm := false

	if len(cliCdRequestData.Account) == 0 {
		promptConfirm = true
		cliCdRequestData.Account = TextInput("Account that you wish to login to:")
	}

	if len(cliCdRequestData.AuthToken) == 0 {
		promptConfirm = true
		cliCdRequestData.AuthToken = TextInput("Provide your api-key:")
	}
	return promptConfirm
}

func PromptConnectorDetails() bool {

	return false
}
