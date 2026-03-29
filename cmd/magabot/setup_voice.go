package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const transcribeVoiceScript = `#!/usr/bin/env python3
"""
Transcribe audio/voice files using faster-whisper.
Usage: transcribe-voice <audio_file>
"""

import sys
import os

def main():
    if len(sys.argv) < 2:
        print("Usage: transcribe-voice <audio_file>", file=sys.stderr)
        sys.exit(1)

    audio_path = sys.argv[1]

    if not os.path.exists(audio_path):
        print(f"File not found: {audio_path}", file=sys.stderr)
        sys.exit(1)

    try:
        from faster_whisper import WhisperModel
    except ImportError:
        print("faster-whisper not installed. Run: pip install faster-whisper", file=sys.stderr)
        sys.exit(1)

    # Use medium model for high accuracy, auto-detect language
    model = WhisperModel("medium", device="cpu", compute_type="int8")

    segments, info = model.transcribe(audio_path, beam_size=5)

    text_parts = []
    for segment in segments:
        text_parts.append(segment.text.strip())

    result = " ".join(text_parts).strip()

    if result:
        print(result)
    else:
        print("[audio tidak dapat ditranskrip]")

if __name__ == "__main__":
    main()
`

const ttsSpeakScript = `#!/usr/bin/env python3
"""
Text-to-speech using Microsoft Edge TTS (free, no API key).
Reads text from stdin, writes OGG Opus audio to stdout.

Usage: echo "Hello" | tts-speak [voice]
Voice defaults to id-ID-GadisNeural (Indonesian female).
See all voices: edge-tts --list-voices

Install: pip install edge-tts
Linux:   sudo apt install ffmpeg
macOS:   brew install ffmpeg
"""

import sys
import asyncio
import io
import platform
import subprocess


def ffmpeg_install_hint():
    if platform.system() == "Darwin":
        return "brew install ffmpeg"
    return "sudo apt install ffmpeg"


VOICE = sys.argv[1] if len(sys.argv) > 1 else "id-ID-GadisNeural"


async def main():
    text = sys.stdin.read().strip()
    if not text:
        sys.exit(1)

    try:
        import edge_tts
    except ImportError:
        print("edge-tts not installed. Run: pip install edge-tts", file=sys.stderr)
        sys.exit(1)

    communicate = edge_tts.Communicate(text, VOICE)
    mp3_buf = io.BytesIO()
    async for chunk in communicate.stream():
        if chunk["type"] == "audio":
            mp3_buf.write(chunk["data"])

    mp3_bytes = mp3_buf.getvalue()
    if not mp3_bytes:
        print("edge-tts produced no audio", file=sys.stderr)
        sys.exit(1)

    # Convert MP3 -> OGG Opus (required for Telegram/WhatsApp voice messages)
    result = subprocess.run(
        [
            "ffmpeg", "-y", "-loglevel", "quiet",
            "-i", "pipe:0",
            "-c:a", "libopus", "-b:a", "32k",
            "-f", "ogg", "pipe:1",
        ],
        input=mp3_bytes,
        capture_output=True,
    )
    if result.returncode != 0:
        print(f"ffmpeg conversion failed: {result.stderr.decode()}", file=sys.stderr)
        print(f"Install ffmpeg: {ffmpeg_install_hint()}", file=sys.stderr)
        sys.exit(1)

    sys.stdout.buffer.write(result.stdout)


asyncio.run(main())
`

// setupVoice installs voice dependencies (faster-whisper, edge-tts, ffmpeg)
// and writes the transcribe-voice and tts-speak scripts to ~/.local/bin/.
// Pass --scripts-only to skip package installation and only write scripts.
func setupVoice() {
	// --scripts-only: just install scripts, used by install.sh after it handles packages
	if len(os.Args) > 3 && os.Args[3] == "--scripts-only" {
		if err := installVoiceScripts(); err != nil {
			fmt.Printf("❌ Failed to install scripts: %v\n", err)
			os.Exit(1)
		}
		return
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n🎤 Voice Setup")
	fmt.Println("──────────────")
	fmt.Println()
	fmt.Println("This will install:")
	fmt.Println("  • faster-whisper  — speech-to-text (runs locally, no API key)")
	fmt.Println("  • edge-tts        — text-to-speech (Microsoft Edge, free)")
	fmt.Println("  • ffmpeg          — audio format conversion (OGG Opus)")
	fmt.Println()
	fmt.Println("After setup, magabot will reply with voice when you send a voice message.")
	fmt.Println()

	if !askYesNo(reader, "Continue?", true) {
		fmt.Println("Canceled.")
		return
	}

	fmt.Println()

	// 1. Install ffmpeg
	installFfmpeg()

	// 2. Install Python packages
	pip := findPip()
	if pip == "" {
		fmt.Println("❌ pip not found. Install Python 3 and pip first.")
		fmt.Println("   https://www.python.org/downloads/")
		return
	}

	installPipPackage(pip, "faster-whisper")
	installPipPackage(pip, "edge-tts")

	// 3. Install scripts
	if err := installVoiceScripts(); err != nil {
		fmt.Printf("❌ Failed to install scripts: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("✅ Voice setup complete!")
	fmt.Println()
	fmt.Println("   Note: faster-whisper will download the 'medium' model (~1.5 GB)")
	fmt.Println("   on first use. Subsequent runs are fast.")
	fmt.Println()
	fmt.Println("   To change TTS voice, edit ~/.local/bin/tts-speak and set VOICE.")
	fmt.Println("   List all voices: edge-tts --list-voices")
}

func installFfmpeg() {
	if commandExists("ffmpeg") {
		fmt.Println("✓ ffmpeg already installed")
		return
	}

	fmt.Print("Installing ffmpeg... ")
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		if !commandExists("brew") {
			fmt.Println()
			fmt.Println("⚠️  Homebrew not found. Install ffmpeg manually:")
			fmt.Println("   brew install ffmpeg")
			return
		}
		cmd = exec.Command("brew", "install", "ffmpeg")
	} else {
		cmd = exec.Command("sudo", "apt-get", "install", "-y", "ffmpeg")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("\n⚠️  Could not install ffmpeg automatically: %v\n", err)
		if runtime.GOOS == "darwin" {
			fmt.Println("   Install manually: brew install ffmpeg")
		} else {
			fmt.Println("   Install manually: sudo apt install ffmpeg")
		}
		return
	}
	fmt.Println("✓")
}

func findPip() string {
	for _, name := range []string{"pip3", "pip"} {
		if commandExists(name) {
			return name
		}
	}
	return ""
}

func installPipPackage(pip, pkg string) {
	fmt.Printf("Installing %s... ", pkg)
	cmd := exec.Command(pip, "install", pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("\n⚠️  Could not install %s: %v\n", pkg, err)
		fmt.Printf("   Install manually: %s install %s\n", pip, pkg)
		return
	}
	fmt.Println("✓")
}

func installVoiceScripts() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0700); err != nil {
		return fmt.Errorf("create ~/.local/bin: %w", err)
	}

	scripts := map[string]string{
		"transcribe-voice": transcribeVoiceScript,
		"tts-speak":        ttsSpeakScript,
	}
	for name, content := range scripts {
		path := filepath.Join(binDir, name)
		if err := os.WriteFile(path, []byte(content), 0700); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
		fmt.Printf("✓ Installed ~/.local/bin/%s\n", name)
	}
	return nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
