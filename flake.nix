{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-23.11";
    xc = {
      url = "github:joerdav/xc";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    virsh-json = {
      url = "github:a-h/virshjson";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { nixpkgs, xc, ... }:
    let
      pkgsForSystem = system: import nixpkgs {
        inherit system;
      };
      allSystems = [
        "x86_64-linux" # 64-bit Intel/AMD Linux
        "aarch64-linux" # 64-bit ARM Linux
        "x86_64-darwin" # 64-bit Intel macOS
        "aarch64-darwin" # 64-bit ARM macOS
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs allSystems (system: f {
        system = system;
        pkgs = pkgsForSystem system;
      });
    in
    {
      # `nix develop` provides a shell containing development tools.
      devShell = forAllSystems ({ system, pkgs }:
        pkgs.mkShell {
          nativeBuildInputs = [
            pkgs.pkg-config # Required to set PKG_CONFIG_PATH to find C libs.
            pkgs.curl
          ];
          buildInputs = [
            pkgs.libvirt
            pkgs.virt-manager
            xc.packages.${system}.xc
            pkgs.pkg-config # Required to set PKG_CONFIG_PATH to find C libs.
          ];
        });
    };
}
