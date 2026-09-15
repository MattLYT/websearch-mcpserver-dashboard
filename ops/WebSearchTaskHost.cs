using System;
using System.Diagnostics;
using System.IO;

// Windows-subsystem entry point: never allocates a console or launches Terminal.
internal static class WebSearchTaskHost
{
    [STAThread]
    private static int Main()
    {
        try
        {
            string directory = AppDomain.CurrentDomain.BaseDirectory;
            string runner = Path.Combine(directory, "run-ensure-websearch.ps1");
            if (!File.Exists(runner)) return 40;
            var info = new ProcessStartInfo
            {
                FileName = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.Windows),
                    @"System32\WindowsPowerShell\v1.0\powershell.exe"),
                Arguments = "-NoProfile -NonInteractive -ExecutionPolicy Bypass -File \"" + runner + "\"",
                WorkingDirectory = Directory.GetParent(directory.TrimEnd(Path.DirectorySeparatorChar)).FullName,
                UseShellExecute = false,
                CreateNoWindow = true
            };
            using (var process = Process.Start(info))
            {
                process.WaitForExit();
                return process.ExitCode;
            }
        }
        catch (Exception error)
        {
            try
            {
                File.AppendAllText(Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "task-host.log"),
                    DateTime.UtcNow.ToString("o") + " launch-failed " + error.GetType().Name + Environment.NewLine);
            }
            catch { /* Preserve a nonzero result even when diagnostics cannot be written. */ }
            return 41;
        }
    }
}
