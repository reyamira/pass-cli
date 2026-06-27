class PassCli < Formula
  desc "Secure CLI password manager with AES-256-GCM encryption"
  homepage "https://github.com/reyamira/pass-cli"
  version "0.8.51"
  license "MIT"

  on_macos do
    on_intel do
      url "https://github.com/reyamira/pass-cli/releases/download/v0.0.1/pass-cli_0.0.1_darwin_amd64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256_FOR_DARWIN_AMD64"
    end
    on_arm do
      url "https://github.com/reyamira/pass-cli/releases/download/v0.0.1/pass-cli_0.0.1_darwin_arm64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256_FOR_DARWIN_ARM64"
    end
  end

  on_linux do
    on_intel do
      url "https://github.com/reyamira/pass-cli/releases/download/v0.0.1/pass-cli_0.0.1_linux_amd64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256_FOR_LINUX_AMD64"
    end
    on_arm do
      url "https://github.com/reyamira/pass-cli/releases/download/v0.0.1/pass-cli_0.0.1_linux_arm64.tar.gz"
      sha256 "REPLACE_WITH_ACTUAL_SHA256_FOR_LINUX_ARM64"
    end
  end

  def install
    bin.install "pass-cli"

    # Generate shell completions
    generate_completions_from_executable(bin/"pass-cli", "completion")

    # Install documentation
    doc.install "README.md" if File.exist?("README.md")
    doc.install "LICENSE" if File.exist?("LICENSE")
  end

  def caveats
    <<~EOS
      Pass-CLI: Secure password manager with TUI and CLI interfaces

      First-time users: Run `pass-cli` (no arguments) for guided setup.

      Quick start:
        pass-cli          - Launch interactive TUI
        pass-cli init     - Initialize vault manually
        pass-cli doctor   - Run health checks

      Features:
        • Interactive TUI for visual management
        • Keychain integration: pass-cli keychain enable
        • Usage tracking and audit logging

      Vault location: ~/.pass-cli/vault.enc
      Complete guide: https://github.com/reyamira/pass-cli/blob/main/docs/GETTING_STARTED.md
    EOS
  end

  test do
    # Test that the binary exists and is executable
    assert_match version.to_s, shell_output("#{bin}/pass-cli version")

    # Test help command
    assert_match "A secure CLI password manager", shell_output("#{bin}/pass-cli --help")

    # Test init command in a temporary directory
    testdir = testpath/"test-vault"
    mkdir_p testdir
    ENV["HOME"] = testdir
    system bin/"pass-cli", "init"
    assert_predicate testdir/".pass-cli", :exist?
  end
end
