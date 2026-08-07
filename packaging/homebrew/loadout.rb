# Homebrew formula for loadout.
#
# Lives in a tap: homebrew-loadout/Formula/loadout.rb, installed with
#
#   brew tap loadoutsh/loadout
#   brew install loadout
#
# The url, sha256 and version fields are rewritten by the release process from
# dist/checksums.txt. The placeholders below are not valid for a real install.
class Loadout < Formula
  desc "Curated shell environment you configure rather than edit"
  homepage "https://github.com/loadoutsh/loadout"
  version "0.0.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/loadoutsh/loadout/releases/download/v#{version}/loadout_v#{version}_darwin_arm64.tar.gz"
      sha256 "REPLACED_AT_RELEASE_DARWIN_ARM64"
    end
    on_intel do
      url "https://github.com/loadoutsh/loadout/releases/download/v#{version}/loadout_v#{version}_darwin_amd64.tar.gz"
      sha256 "REPLACED_AT_RELEASE_DARWIN_AMD64"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/loadoutsh/loadout/releases/download/v#{version}/loadout_v#{version}_linux_arm64.tar.gz"
      sha256 "REPLACED_AT_RELEASE_LINUX_ARM64"
    end
    on_intel do
      url "https://github.com/loadoutsh/loadout/releases/download/v#{version}/loadout_v#{version}_linux_amd64.tar.gz"
      sha256 "REPLACED_AT_RELEASE_LINUX_AMD64"
    end
  end

  def install
    bin.install "loadout"
    generate_completions_from_executable(bin/"loadout", "completion")
  end

  def caveats
    <<~EOS
      Get started with:
        loadout init
        loadout apply

      Then add the generated file to your shell config:
        source ~/.config/loadout/init.zsh

      loadout does not edit your shell config for you.
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/loadout version")
    # The catalog is embedded, so this works with no configuration present.
    assert_match "aliases:", shell_output("#{bin}/loadout catalog")
  end
end
