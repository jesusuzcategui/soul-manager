package main

import (
	"fmt"

	_ "fyne.io/fyne/v2"
	_ "github.com/shirou/gopsutil/v3/net"
	_ "github.com/shirou/gopsutil/v3/process"

	"github.com/jesusuzcategui/soul-app/internal/model"
)

func main() {
	fmt.Println("Soul App skeleton initialized.")
	_ = model.PortProcess{}
}
