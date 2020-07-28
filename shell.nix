with import <nixpkgs> { };


stdenv.mkDerivation {
  name = "go";
  buildInputs = [ libcap go gcc bash glibcLocales ];
  LOCALE_ARCHIVE_2_27="${pkgs.glibcLocales}/lib/locale/locale-archive";
  shellHook = "";
}
