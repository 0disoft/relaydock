package main

/* llmnav/1 module
id=relaydock.command.releasepack
role=Audit, build, sign, and verify RelayDock release bundles, checksums, readiness evidence, and update manifests.
owns=release bundle CLI|checksum signature commands|update manifest verification
excludes=GitHub publication|runtime update installation
search=build RelayDock release|sign release checksums|verify update manifest
invariant=Signing and verification are explicit subcommands over caller-selected release artifacts rather than implicit build side effects.
stability=contract
*/

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/0disoft/relaydock/internal/releasepack"
	"github.com/0disoft/relaydock/internal/updater"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "audit":
		err = runAudit(os.Args[2:])
	case "verify":
		err = runVerify(os.Args[2:])
	case "build":
		err = runBuild(os.Args[2:])
	case "readiness":
		err = runReadiness(os.Args[2:])
	case "sign-checksums":
		err = runSignChecksums(os.Args[2:])
	case "verify-checksums":
		err = runVerifyChecksums(os.Args[2:])
	case "sign-update-manifest":
		err = runSignUpdateManifest(os.Args[2:])
	case "verify-update-manifest":
		err = runVerifyUpdateManifest(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runSignUpdateManifest(arguments []string) error {
	flags := flag.NewFlagSet("sign-update-manifest", flag.ContinueOnError)
	input := flags.String("input", "", "unsigned updater manifest JSON")
	output := flags.String("output", "", "signed updater manifest output path")
	privateKeyEnvironment := flags.String("private-key-env", "RELAYDOCK_UPDATER_SIGNING_PRIVATE_KEY", "environment variable containing the base64 Ed25519 updater key")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *input == "" || *output == "" {
		return fmt.Errorf("sign-update-manifest requires --input and --output")
	}
	manifest, err := updater.ReadManifestFile(*input)
	if err != nil {
		return err
	}
	privateKey, err := releasepack.DecodeReleasePrivateKey(os.Getenv(*privateKeyEnvironment))
	if err != nil {
		return err
	}
	signed, err := updater.SignManifest(manifest, privateKey)
	if err != nil {
		return err
	}
	if err := updater.WriteManifestFile(*output, signed); err != nil {
		return err
	}
	fmt.Printf("update manifest signed: version=%s output=%s\n", signed.Version, *output)
	return nil
}

func runVerifyUpdateManifest(arguments []string) error {
	flags := flag.NewFlagSet("verify-update-manifest", flag.ContinueOnError)
	input := flags.String("input", "", "signed updater manifest JSON")
	publicKeyEnvironment := flags.String("public-key-env", "RELAYDOCK_UPDATER_SIGNING_PUBLIC_KEY", "environment variable containing the base64 Ed25519 updater public key")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *input == "" {
		return fmt.Errorf("verify-update-manifest requires --input")
	}
	manifest, err := updater.ReadManifestFile(*input)
	if err != nil {
		return err
	}
	publicKey, err := releasepack.DecodeReleasePublicKey(os.Getenv(*publicKeyEnvironment))
	if err != nil {
		return err
	}
	if err := updater.VerifyManifestSignature(manifest, publicKey); err != nil {
		return err
	}
	fmt.Printf("update manifest verified: version=%s input=%s\n", manifest.Version, *input)
	return nil
}

func runSignChecksums(arguments []string) error {
	flags := flag.NewFlagSet("sign-checksums", flag.ContinueOnError)
	input := flags.String("input", "", "checksum file to sign")
	output := flags.String("output", "", "signature envelope output path")
	privateKeyEnvironment := flags.String("private-key-env", "RELAYDOCK_RELEASE_SIGNING_PRIVATE_KEY", "environment variable containing the base64 Ed25519 release key")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *input == "" || *output == "" {
		return fmt.Errorf("sign-checksums requires --input and --output")
	}
	privateKey, err := releasepack.DecodeReleasePrivateKey(os.Getenv(*privateKeyEnvironment))
	if err != nil {
		return err
	}
	envelope, err := releasepack.SignReleaseChecksums(*input, privateKey)
	if err != nil {
		return err
	}
	if err := releasepack.WriteReleaseSignature(*output, envelope); err != nil {
		return err
	}
	fmt.Printf("release checksums signed: keyId=%s output=%s\n", envelope.KeyID, *output)
	return nil
}

func runVerifyChecksums(arguments []string) error {
	flags := flag.NewFlagSet("verify-checksums", flag.ContinueOnError)
	input := flags.String("input", "", "signed checksum file")
	signature := flags.String("signature", "", "signature envelope path")
	publicKeyEnvironment := flags.String("public-key-env", "RELAYDOCK_RELEASE_SIGNING_PUBLIC_KEY", "environment variable containing the base64 Ed25519 release public key")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *input == "" || *signature == "" {
		return fmt.Errorf("verify-checksums requires --input and --signature")
	}
	publicKey, err := releasepack.DecodeReleasePublicKey(os.Getenv(*publicKeyEnvironment))
	if err != nil {
		return err
	}
	envelope, err := releasepack.ReadReleaseSignature(*signature)
	if err != nil {
		return err
	}
	if err := releasepack.VerifyReleaseChecksums(*input, envelope, publicKey); err != nil {
		return err
	}
	fmt.Printf("release checksums verified: keyId=%s input=%s\n", envelope.KeyID, *input)
	return nil
}

func runReadiness(arguments []string) error {
	flags := flag.NewFlagSet("readiness", flag.ContinueOnError)
	root := flags.String("root", ".", "repository root")
	version := flags.String("version", "", "expected release version")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	report, err := releasepack.CheckReleaseReadiness(*root, *version)
	if err != nil {
		return err
	}
	fmt.Printf("release ready: version=%s module=%s files=%d\n", report.Version, report.Module, report.Files)
	return nil
}

func runAudit(arguments []string) error {
	flags := flag.NewFlagSet("audit", flag.ContinueOnError)
	root := flags.String("root", ".", "repository root")
	maximum := flags.Int64("max-bytes", releasepack.DefaultMaximumFileBytes, "maximum regular file size")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	report, err := releasepack.Audit(*root, *maximum)
	if err != nil {
		return err
	}
	fmt.Printf("audit ok: files=%d totalBytes=%d largest=%s (%d bytes)\n", report.Files, report.TotalBytes, report.LargestFilePath, report.LargestFileBytes)
	return nil
}

func runVerify(arguments []string) error {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	root := flags.String("root", ".", "repository root")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	report, err := releasepack.Verify(*root)
	if err != nil {
		return err
	}
	fmt.Printf("manifest ok: files=%d chunks=%d totalBytes=%d largest=%s (%d bytes)\n", report.Files, report.ManifestChunks, report.TotalBytes, report.LargestFilePath, report.LargestFileBytes)
	return nil
}

func runBuild(arguments []string) error {
	flags := flag.NewFlagSet("build", flag.ContinueOnError)
	root := flags.String("root", ".", "repository root")
	output := flags.String("output", "", "output ZIP path")
	prefix := flags.String("prefix", "relaydock", "archive root directory")
	generated := flags.String("generated-at", "", "RFC3339 archive timestamp")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *output == "" {
		version, err := os.ReadFile(filepath.Join(*root, "VERSION"))
		if err != nil {
			return fmt.Errorf("read VERSION: %w", err)
		}
		*output = filepath.Join(filepath.Dir(filepath.Clean(*root)), "relaydock-"+stringTrimSpace(version)+".zip")
	}
	var generatedAt time.Time
	var err error
	if *generated != "" {
		generatedAt, err = time.Parse(time.RFC3339, *generated)
		if err != nil {
			return fmt.Errorf("parse generated-at: %w", err)
		}
	}
	report, err := releasepack.Build(releasepack.BuildOptions{
		Root: *root, OutputZIP: *output, ArchivePrefix: *prefix, GeneratedAt: generatedAt,
	})
	if err != nil {
		return err
	}
	fmt.Printf("archive built: %s\nsha256: %s\nfiles: %d manifestChunks: %d bytes: %d\n", report.OutputZIP, report.ArchiveSHA256, report.Files, report.ManifestChunks, report.ArchiveBytes)
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: releasepack <audit|verify|build|readiness|sign-checksums|verify-checksums|sign-update-manifest|verify-update-manifest> [flags]")
}

func stringTrimSpace(value []byte) string {
	start, end := 0, len(value)
	for start < end && (value[start] == ' ' || value[start] == '\n' || value[start] == '\r' || value[start] == '\t') {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\n' || value[end-1] == '\r' || value[end-1] == '\t') {
		end--
	}
	return string(value[start:end])
}
