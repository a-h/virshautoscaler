package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/a-h/virshautoscaler/hypervisor"
	"github.com/a-h/virshautoscaler/sloghandler"
)

var flagVerbose = flag.Bool("v", false, "Set to true for verbose logs")

func main() {
	flag.Parse()

	var addSource bool
	level := slog.LevelInfo
	if *flagVerbose {
		addSource = true
		level = slog.LevelDebug
	}

	log := slog.New(sloghandler.NewHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: addSource,
		Level:     level,
	}))

	h, err := hypervisor.New()
	if err != nil {
		log.Error("Failed to connect to hypervisor", slog.Any("error", err))
		return
	}
	defer h.Close()

	log.Debug("Listing VMs")
	domainNameToStatus := make(map[string]hypervisor.State)
	domains, err := h.List()
	if err != nil {
		log.Error("Failed to list VMs", slog.Any("error", err))
		return
	}
	for _, d := range domains {
		domainNameToStatus[d.Name] = d.State
	}
	log.Debug("Current VMs", slog.Any("domains", domains))

	machines := []*hypervisor.Machine{
		hypervisor.NewMachine("runner-1", "/vm/runner-1.qcow2"),
		hypervisor.NewMachine("runner-2", "/vm/runner-2.qcow2"),
	}
	log.Debug("Ensuring that expected machines are started", slog.Any("machines", machines))

	var hadErrors bool
	for i := 0; i < len(machines); i++ {
		status := domainNameToStatus[machines[i].Name]
		if status == hypervisor.StateRunning {
			log.Debug("Machine is already started, skipping", slog.String("name", machines[i].Name))
			continue
		}
		_, err := h.Create(machines[i])
		if err != nil {
			hadErrors = true
			log.Error("Failed to create machine", slog.String("name", machines[i].Name), slog.Any("error", err))
			continue
		}
		log.Info("Created machine", slog.String("name", machines[i].Name))
	}
	if hadErrors {
		log.Debug("Exiting with non-zero exit code due to errors")
		os.Exit(1)
	}
	log.Info("Completed successfully")
}
