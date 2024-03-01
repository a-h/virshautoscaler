# virshautoscaler

Script to start and stop VMs.

## Setup

### Install Nix

A prerequisite for getting started is to install Nix, via the Determinate Systems Installer.

Use Nix 2.18.1 to match the version used inside the VM itself, or you will see a hash mismatch error.

```bash
wget https://github.com/DeterminateSystems/nix-installer/releases/download/v0.16.1/nix-installer-x86_64-linux
chmod +x ./nix-installer-x86_64-linux
./nix-installer-x86_64-linux install --nix-package-url=https://releases.nixos.org/nix/nix-2.18.1/nix-2.18.1-x86_64-linux.tar.xz
```

### Install libvirt

While the Nix configuration does most of the work, the Linux machine will need to have various network interfaces and bridges configured by installing `libvirt` and `qemu-kvm`.

```bash
sudo yum install -y libvirt qemu-kvm virt-install
```

### Give your user access to run VMs

```bash
sudo usermod -a -G libvirt $USER
newgrp libvirt
```

### Ensure that libvirt runs VMs as libvirt

Find `group=root` and replace with `group=libvirt` (ensure it's not commented out).

```bash
vi /etc/libvirt/qemu.conf
```

### Restart libvirtd

```bash
sudo systemctl restart libvirtd
```

### List VMs to check

```bash
export LIBVIRT_DEFAULT_URI=qemu:///system
virsh list --all
```

You can verify that you have the pre-requisites, by running `ifconfig`. If you can see `virbr0`, then you have the default VM network, so `qemu` is likely properly configured on your OS.

### Adjust disk space

On Azure, you might need to expand your disks, and also get rid of the tiny `/tmp` partition and replace it with a directory in `/tmp`.

Expand the disk.

```bash
sudo growpart /dev/sda 3
sudo pvresize /dev/sda3
```

Extend the root and home volumes

```bash
sudo lvextend -r -l +60%FREE /dev/mapper/vg1-root
sudo lvextend -r -l +60%FREE /dev/mapper/vg1-home
```

Move the `/tmp` dir to the root partition by unmounting the `/tmp` partition.

```bash
sudo umount /tmp
sudo chmod 1777 /tmp
```

Make it permanent by updating the `/etc/fstab` to comment out the `/tmp` partition:

```
/dev/mapper/vg1-root    /                       xfs     defaults        0 0
UUID=fb2f277f-9022-45b5-a601-d3a856bb82dd /boot                   xfs     defaults,nodev,noexec,nosuid 0 0
UUID=009B-9A47          /boot/efi               vfat    defaults,uid=0,gid=0,umask=077,shortname=winnt 0 2
/dev/mapper/vg1-home    /home                   xfs     defaults,nodev,nosuid 0 0
# Comment out this line.
#/dev/mapper/vg1-tmp     /tmp                    xfs     defaults,nodev,noexec,nosuid 0 0
/dev/mapper/vg1-var     /var                    xfs     defaults,nodev,nosuid 0 0
/dev/mapper/vg1-var_log /var/log                xfs     defaults,nodev,noexec,nosuid 0 0
/dev/mapper/vg1-var_log_audit /var/log/audit          xfs     defaults,nodev,noexec,nosuid 0 0

# CIS L2 v1.0.1 - 1.1.7 - 1.1.10 and Lynis harden /dev/shm
/tmp /var/tmp none rw,noexec,nosuid,nodev,bind 0 0

# CIS L2 v1.0.1 - 1.1.15 - 1.1.17 harden /dev/shm
tmpfs /dev/shm tmpfs defaults,nodev,nosuid,noexec 0 0
/dev/disk/cloud/azure_resource-part1    /mnt    auto    defaults,nofail,x-systemd.requires=cloud-init.service,comment=cloudconfig       0       2
```

### Clone repo

Following that, clone this Github Repo to build the VMs.

```bash
nix shell nixpkgs#git
git clone https://github.com/virshautoscaler/github-runner-nix
```

You can use a Github Personal Access token as a password instead of loading your private SSH keys onto a server, using:

```bash
export GITHUB_PAT=gh_pat_example
git clone https://$GITHUB_PAT@github.com/virshautoscaler/github-runner-nix
```

### Run `nix develop`

Inside the cloned repo, run `nix develop`. This will provide a shell with all development tools installed.

### Run the build steps

Inside the `nix develop` shell, you will have `xc` and other tools that can execute the tasks, e.g. `build-images`.

### Setup secrets

The manager requires that a directory called `metadata` exists.

```bash
mkdir -p metadata/secrets
```

Create a file called `github_pat` in that directory containing the Github PAT required to start runners.

## Tasks

### build-image-1

If you add more, add more builds, one for each host.

```bash
sudo mkdir -p /vm
sudo chown -R $USER:libvirt /vm

nix build ./#vms.runner-1
sudo cp -L ./result/nixos.img /vm/runner-1.img

sudo chmod 660 /vm/*.img
# For Rocky.
sudo chown -R $USER:libvirt /vm
# For Ubuntu.
# sudo chown -R libvirt-qemu:libvirt-qemu /vm
```

### build-images

If you add more, add more builds, one for each host.

```bash
sudo mkdir -p /vm
sudo chown -R $USER:libvirt /vm

nix build ./#vms.runner-1
sudo cp -L ./result/nixos.img /vm/runner-1.img

nix build ./#vms.runner-2
sudo cp -L ./result/nixos.img /vm/runner-2.img

sudo chmod 660 /vm/*.img
# For Rocky.
sudo chown -R $USER:libvirt /vm
# For Ubuntu.
# sudo chown -R libvirt-qemu:libvirt-qemu /vm
```

### virt-run

Env: LIBVIRT_DEFAULT_URI=qemu:///system

Copy the image from the read-only Nix store to the local directory, and run it.

```bash
sudo mkdir -p /vm
sudo cp -L ./result/runner-1.img /vm
sudo chmod 660 /vm/runner-1.img
sudo chown -R libvirt-qemu:libvirt-qemu /vm
virt-install --name runner-1 --memory 2048 --vcpus 1 --disk /vm/runner-1.img,bus=sata --import --os-variant nixos-unknown --network default --noautoconsole
```

### virt-list

Env: LIBVIRT_DEFAULT_URI=qemu:///system

```bash
virsh list --all
```

### virt-kill-all

Shutdown with virtsh shutdown, or in this case, completely remove it with undefine.

Env: LIBVIRT_DEFAULT_URI=qemu:///system

```bash
virsh destroy runner-1 || true
virsh undefine runner-1 --remove-all-storage || true
virsh destroy runner-2 || true
virsh undefine runner-2 --remove-all-storage || true
```

### virt-ssh

https://www.cyberciti.biz/faq/find-ip-address-of-linux-kvm-guest-virtual-machine/

On hardened machines, you may need to run `ssh -v -o KexAlgorithms=curve25519-sha256 user@ip` because the default options are NIST curves.

```bash
virsh domifaddr runner-1 | virsh-json | jq -r ".[0].Address"
```

### firewall-config-ubuntu

On the host machine, to allow the VMs to access your machine, run:

```bash
sudo ufw allow from 192.168.122.0/16 to 192.168.122.1 port 9494 proto tcp
sudo ufw reload
```

### firewall-config-rocky

Rocky Linux uses firewalld instead of ufw, so the command is slightly different.

```bash
sudo firewall-cmd --permanent --zone=internal --add-port=9494/tcp
sudo firewall-cmd --permanent --zone=internal --add-interface=virb0
sudo firewall-cmd --permanent --zone=internal --add-source=192.168.122.0/16
sudo firewall-cmd --reload
```

### run-manager

```bash
go run ./cmd/github-runner-manager/. -v -vm /vm/runner-1.img -vm /vm/runner-2.img
```

### virsh-dumpxml

To see the underlying XML of a domain, you can dump it.

```bash
virsh dumpxml runner-1  > config.xml
```

### nix-store-garbage-collection

Run this if diskspace is an issue. will remove old packages.

```bash
# clean up nix store
nix-store --gc
```
