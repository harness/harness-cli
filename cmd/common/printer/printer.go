package printer

import (
	"fmt"
	"harness/config"
)

func Print(res any, pageIndex, pageCount, itemCount int64, printCountInfo bool) error {
	var err error
	if config.Global.Format == "json" {
		err = PrintJson(res, pageIndex, pageCount, itemCount)
	} else {
		err = PrintTable(res, pageIndex, pageCount, itemCount)
	}

	if err != nil {
		fmt.Println(err)
		return err
	}

	if printCountInfo {
		fmt.Printf("Page %d of %d (Total: %d)\n",
			pageIndex, pageCount, itemCount)
	}
	return nil
}
