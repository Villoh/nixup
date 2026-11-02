{
  description = "nixup: update checker and safe updater for flake-based NixOS";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAll = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAll (pkgs: rec {
        nixup = pkgs.buildGoModule {
          pname = "nixup";
          version = "0.1.0";
          src = self;
          vendorHash = null; # standard library only
          ldflags = [
            "-s"
            "-w"
            "-X main.version=${self.shortRev or self.dirtyShortRev or "dev"}"
          ];
          # The tools it shells out to; PATH is left alone so the system's own nh/nix win.
          meta = {
            description = "Update checker and safe updater for flake-based NixOS";
            mainProgram = "nixup";
          };
        };
        default = nixup;
      });

      overlays.default = _final: prev: {
        nixup = self.packages.${prev.stdenv.hostPlatform.system}.nixup;
      };

      devShells = forAll (pkgs: {
        default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.gopls
            pkgs.nix-update
          ];
        };
      });
    };
}
