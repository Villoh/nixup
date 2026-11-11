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
      version = "0.7.1"; # the one place the version lives; the release workflow bumps it
    in
    {
      packages = forAll (pkgs: rec {
        nixup = pkgs.buildGoModule {
          pname = "nixup";
          inherit version;
          src = self;
          subPackages = [ "cmd/nixup" ];
          vendorHash = null; # standard library only
          nativeCheckInputs = [ pkgs.git ]; # one test runs against a real repository
          nativeBuildInputs = [ pkgs.installShellFiles ];
          # Shell completions, printed by the binary itself (needs to run it: not when cross-compiling).
          postInstall = pkgs.lib.optionalString (pkgs.stdenv.buildPlatform.canExecute pkgs.stdenv.hostPlatform) ''
            installShellCompletion --cmd nixup \
              --bash <($out/bin/nixup completion bash) \
              --zsh <($out/bin/nixup completion zsh) \
              --fish <($out/bin/nixup completion fish)
          '';
          ldflags = [
            "-s"
            "-w"
            "-X github.com/Villoh/nixup/internal/cli.Version=${version}"
            "-X github.com/Villoh/nixup/internal/cli.Commit=${self.shortRev or self.dirtyShortRev or ""}"
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
