# Homebrew formula for okashi — builds from source.
#
# Replace OWNER below if your GitHub user/org isn't "snackztime", and bump
# `url`/`sha256` on each release (the release workflow prints the sha256 to
# paste here — see .github/workflows/release.yml).
class Okashi < Formula
  desc "Minimal, distraction-free terminal writing app"
  homepage "https://github.com/snackztime/okashi"
  url "https://github.com/snackztime/okashi/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"
  license "MIT"
  head "https://github.com/snackztime/okashi.git", branch: "main"

  depends_on "go" => :build

  def install
    # Inject the release version (Formula#version is parsed from the url tag)
    # so `okashi --version` reports the installed release. -s -w strips the
    # symbol table for a smaller binary.
    ldflags = "-s -w -X main.version=#{version}"
    system "go", "build", *std_go_args(ldflags: ldflags)
  end

  test do
    # okashi is a TUI with no non-interactive run mode, so exercise the
    # deterministic --version path instead of launching the interface.
    assert_match "okashi #{version}", shell_output("#{bin}/okashi --version")
  end
end
