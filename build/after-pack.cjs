const { execFileSync } = require('child_process');
const path = require('path');
const fs = require('fs');

module.exports = async function afterPack(context) {
  if (context.electronPlatformName !== 'win32') {
    return;
  }

  const executableName = `${context.packager.appInfo.productFilename}.exe`;
  const executablePath = path.join(context.appOutDir, executableName);
  const tempExecutablePath = path.join(context.appOutDir, 'GistManager.exe');
  const iconPath = path.join(context.packager.projectDir, 'build', 'icon.ico');
  const rcEditPath = path.join(
    context.packager.projectDir,
    'node_modules',
    'electron-winstaller',
    'vendor',
    'rcedit.exe',
  );

  if (!fs.existsSync(executablePath)) {
    throw new Error(`Windows executable not found: ${executablePath}`);
  }

  if (!fs.existsSync(iconPath)) {
    throw new Error(`Icon file not found: ${iconPath}`);
  }

  if (!fs.existsSync(rcEditPath)) {
    throw new Error(`rcedit not found: ${rcEditPath}`);
  }

  let rcEditTarget = executablePath;
  let renameBack = false;

  if (/[^\x00-\x7F]/.test(executablePath)) {
    fs.renameSync(executablePath, tempExecutablePath);
    rcEditTarget = tempExecutablePath;
    renameBack = true;
  }

  try {
    execFileSync(rcEditPath, [rcEditTarget, '--set-icon', iconPath], {
      stdio: 'inherit',
    });
  } finally {
    if (renameBack) {
      fs.renameSync(tempExecutablePath, executablePath);
    }
  }
};
