let
  pkgs = import <nixpkgs> {};
in
pkgs.mkShell {
  nativeBuildInputs = with pkgs; [
    go
    go-outline
    gopls
    gopkgs
    go-tools
    delve
  ];

  hardeningDisable = ["all"];
}
