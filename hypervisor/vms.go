package hypervisor

import (
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

type Hypervisor struct {
	Log    *slog.Logger
	Client *libvirt.Connect
}

func New(log *slog.Logger) (h *Hypervisor, err error) {
	h = &Hypervisor{
		Log: log,
	}
	h.Client, err = libvirt.NewConnect("qemu:///system")
	return h, err
}

type State string

const StateNone State = "NOSTATE"
const StateRunning State = "RUNNING"
const StateBlocked State = "BLOCKED"
const StatePaused State = "PAUSED"
const StateShutdown State = "SHUTDOWN"
const StateCrashed State = "CRASHED"
const StateSuspended State = "PMSUSPENDED"
const StateShutoff State = "SHUTOFF"

var virStateToName = map[libvirt.DomainState]State{
	libvirt.DOMAIN_NOSTATE:     StateNone,
	libvirt.DOMAIN_RUNNING:     StateRunning,
	libvirt.DOMAIN_BLOCKED:     StateBlocked,
	libvirt.DOMAIN_PAUSED:      StatePaused,
	libvirt.DOMAIN_SHUTDOWN:    StateShutdown,
	libvirt.DOMAIN_CRASHED:     StateCrashed,
	libvirt.DOMAIN_PMSUSPENDED: StateSuspended,
	libvirt.DOMAIN_SHUTOFF:     StateShutoff,
}

type Domain struct {
	Name  string
	UUID  string
	State State
	Addrs []string
}

func newDomain(vd libvirt.Domain) (d Domain, err error) {
	d.Name, err = vd.GetName()
	if err != nil {
		return d, fmt.Errorf("failed to get VM Name: %v", err)
	}
	d.UUID, err = vd.GetUUIDString()
	if err != nil {
		return d, fmt.Errorf("failed to get VM UUID: %v", err)
	}
	s, _, _ := vd.GetState()
	d.State = virStateToName[s]

	ifs, err := vd.ListAllInterfaceAddresses(libvirt.DOMAIN_INTERFACE_ADDRESSES_SRC_LEASE)
	if err != nil {
		return d, fmt.Errorf("failed to lookup interface addresses: %w", err)
	}
	for _, in := range ifs {
		for _, addr := range in.Addrs {
			d.Addrs = append(d.Addrs, addr.Addr)
		}
	}
	return d, nil
}

func (h *Hypervisor) List() (vms []Domain, err error) {
	doms, err := h.Client.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE & libvirt.CONNECT_LIST_DOMAINS_INACTIVE)
	if err != nil {
		return vms, fmt.Errorf("failed to list: %w", err)
	}

	vms = make([]Domain, len(doms))
	for i := 0; i < len(doms); i++ {
		vms[i], err = newDomain(doms[i])
		doms[i].Free()
	}

	return vms, nil
}

func (h *Hypervisor) Get(name string) (vm Domain, ok bool, err error) {
	dom, err := h.Client.LookupDomainByName(name)
	if err != nil {
		return vm, false, fmt.Errorf("failed to lookup domain by name: %w", err)
	}
	defer dom.Free()
	if dom == nil {
		return
	}
	vm, err = newDomain(*dom)
	return vm, true, err
}

type Machine struct {
	Name             string
	MemoryMB         int
	VCPU             int
	Architecture     string
	BootDiskFileName string
	Network          string
}

func (m Machine) RuntimeBootDiskFileName() string {
	return strings.TrimSuffix(m.BootDiskFileName, ".img") + "_run.img"
}

func NewMachine(name string, bootDiskFileName string) Machine {
	return Machine{
		Name:             name,
		MemoryMB:         1024,
		VCPU:             1,
		Architecture:     "x86_64",
		BootDiskFileName: bootDiskFileName,
		Network:          "default",
	}
}

func copyFile(src, dst string) (int64, error) {
	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()

	// Use a larger buffer.
	buf := make([]byte, 1024*1024*5) // 5MB buffer.
	nBytes, err := io.CopyBuffer(destination, source, buf)
	return nBytes, err
}

// Create a transient domain (one that can't be restarted, paused or stopped).
// A transient domain is automatically undefined when it stops.
func (h *Hypervisor) Create(m Machine) (d Domain, err error) {
	h.Log.Debug("Copying boot disk", slog.String("src", m.BootDiskFileName), slog.String("tgt", m.RuntimeBootDiskFileName()))
	_, err = copyFile(m.BootDiskFileName, m.RuntimeBootDiskFileName())
	if err != nil {
		return d, fmt.Errorf("failed to copy file: %v", err)
	}
	var domainXML libvirtxml.Domain
	domainXML.Name = m.Name
	domainXML.Type = "kvm"
	domainXML.Memory = &libvirtxml.DomainMemory{
		Value: uint(m.MemoryMB),
		Unit:  "MiB",
	}
	domainXML.VCPU = &libvirtxml.DomainVCPU{
		Placement: "static",
		Value:     uint(m.VCPU),
	}
	domainXML.OS = &libvirtxml.DomainOS{
		Type: &libvirtxml.DomainOSType{
			Arch: m.Architecture,
			Type: "hvm",
		},
	}
	domainXML.Devices = &libvirtxml.DomainDeviceList{}
	// Log to serial, if you want.
	//  domainXML.Devices.Serials = append(domainXML.Devices.Serials, libvirtxml.DomainSerial{
	//    Source: &libvirtxml.DomainChardevSource{
	//      File: &libvirtxml.DomainChardevSourceFile{
	//        //TODO: The name might include path characters, which would be bad.
	//        Path:     "/var/log/libvirt/qemu/serial-" + args.Name + ".log",
	//        SecLabel: []libvirtxml.DomainDeviceSecLabel{},
	//      },
	//    },
	//  })
	domainXML.Devices.Disks = append(domainXML.Devices.Disks, libvirtxml.DomainDisk{
		Device: "disk",
		Driver: &libvirtxml.DomainDiskDriver{
			Name: "qemu",
			Type: "raw",
		},
		Source: &libvirtxml.DomainDiskSource{
			File: &libvirtxml.DomainDiskSourceFile{
				File: m.RuntimeBootDiskFileName(),
			},
		},
		Target: &libvirtxml.DomainDiskTarget{
			Dev: "sda",
			Bus: "sata",
		},
	})
	domainXML.Devices.Interfaces = append(domainXML.Devices.Interfaces, libvirtxml.DomainInterface{
		Source: &libvirtxml.DomainInterfaceSource{
			Network: &libvirtxml.DomainInterfaceSourceNetwork{
				Network: m.Network,
			},
		},
		Model: &libvirtxml.DomainInterfaceModel{
			Type: "virtio",
		},
	})

	dxml, err := xml.MarshalIndent(domainXML, "", " ")
	if err != nil {
		return d, fmt.Errorf("failed to create XML: %w", err)
	}
	h.Log.Info("Starting machine", slog.String("name", m.Name))
	vd, err := h.Client.DomainCreateXML(string(dxml), libvirt.DOMAIN_NONE)
	if err != nil {
		return d, fmt.Errorf("failed to create domain: %v", err)
	}
	defer vd.Free()
	h.Log.Info("Started machine", slog.String("name", m.Name))
	return newDomain(*vd)
}

func uintPtr[T ~int](v T) *uint {
	x := uint(v)
	return &x
}

// Destroy a domain. This immediately stops the machine with a power off.
func (h *Hypervisor) Destroy(name string) (err error) {
	dom, err := h.Client.LookupDomainByName(name)
	if err != nil {
		return fmt.Errorf("failed to lookup domain by name: %w", err)
	}
	if dom == nil {
		return
	}
	defer dom.Free()
	return dom.Destroy()
}

func (h *Hypervisor) Close() (err error) {
	_, err = h.Client.Close()
	return err
}
