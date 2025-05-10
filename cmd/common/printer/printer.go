package printer

import (
	"fmt"
	"harness/config"
)

func Print(res any, pageIndex, pageCount, itemCount int64) error {
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

	return nil
}
