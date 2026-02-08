# FAQ

This section collects common questions and solutions encountered while using cdpnetool.

---

## Q: Connection fails when clicking the "Connect" button?

**Possible Causes:**
- Browser not started or incorrect DevTools URL
- Port occupied or blocked by firewall
- Chrome browser remote debugging not enabled

**Solutions:**
1. Prioritize using the "Launch Browser" button to let the tool automatically launch Chrome
2. Check if DevTools URL format is correct (`http://localhost:port`)
3. If manually launching browser, ensure the `--remote-debugging-port=9222` parameter is added
4. Check if firewall is blocking the port

---

## Q: Rules not working, requests not being intercepted?

**Troubleshooting Steps:**
1. Confirm configuration is enabled (switch on in configuration list, showing "· Running")
2. Confirm target page is attached (showing "Attached" in Targets panel)
3. Confirm the rule itself is enabled (rule's `enabled` is true)
4. Check if matching conditions are correct:
   - Does URL condition match actual request
   - Is the lifecycle stage (request/response) selected correctly
5. Switch to "Events" panel to view "Unmatched Requests" list to confirm if request is captured

**Debugging Tips:**
- First create a simple rule (e.g., only match URL containing a keyword) to verify basic flow
- Use browser DevTools Network panel to view actual request URL
- Gradually add matching conditions to narrow down the scope

---

## Q: Regular expression not working or reporting error?

**Common Errors:**
- Incorrect use of escape characters (double escaping required in JSON)
- Regex syntax errors

**Solutions:**
1. Use online regex testing tools (such as regex101.com) to validate expressions
2. In JSON, backslashes need to be escaped, for example `\d` should be written as `\\d`
3. Example: Match URL ending with number
   ```json
   {"type": "urlRegex", "pattern": "/api/user/\\d+$"}
   ```

---

## Q: Rules not updated after modifying configuration?

**Reasons:**
- Configuration not saved after modification
- Configuration saved but not re-enabled

**Solutions:**
1. Press `Ctrl+S` or `Cmd+S` to save configuration
2. If configuration is in enabled state, saving will automatically reload rules
3. If rules still not effective, try disabling and then re-enabling configuration

---

## Q: Interception stopped after switching attached target?

**Explanation:**
This is expected behavior. cdpnetool uses single-target mode, when switching targets it will automatically:
1. Detach old target
2. Stop interception
3. Attach new target

**Solution:**
After switching targets, need to click the configuration switch again to enable interception.

---

## Q: Events panel not showing any events?

**Troubleshooting Steps:**
1. Confirm browser is connected and target is attached
2. Confirm configuration is enabled (interception is on)
3. Refresh page or trigger request in browser
4. Check "Unmatched Requests" list to confirm if request is captured

If there is data in "Unmatched Requests", it means interception is working normally, just rules not matching.

---

## Q: JSON Patch operation failed or not working?

**Common Reasons:**
- Incorrect JSON Path
- Target field does not exist
- Wrong operation type selected

**Solutions:**
1. View actual response body structure in Events panel
2. Confirm JSON Path format is correct (use `/` separator, array uses numeric index)
3. Use `add` operation to create non-existent fields
4. Use `replace` operation only to modify existing fields

**Example:**
```json
{
  "type": "patchBodyJson",
  "patches": [
    {"op": "replace", "path": "/data/user/name", "value": "newName"}
  ]
}
```

---

## Q: Configuration file import failed?

**Possible Reasons:**
- JSON format error (missing quotes, commas, etc.)
- Configuration version incompatible
- File encoding issue

**Solutions:**
1. Use JSON validation tool to check format (such as jsonlint.com)
2. Confirm configuration contains required fields (id, name, version, rules)
3. Try manually creating the same rule in visual editor

---

## Q: Cookie modification not working?

**Possible Cause:**

This may be a known limitation in Chromium. In older versions of Chrome/Chromium, the CDP (Chrome DevTools Protocol) `Fetch.continueRequest` method cannot modify the Cookie Header.

**Solutions:**

1. **Upgrade Browser Version**:
   - This issue has been fixed in newer versions ([Chromium Commit #7077239](https://chromium-review.googlesource.com/c/chromium/src/+/7077239))
   - Recommend using Chrome 145+ or newer stable versions

2. **Use Canary Version** (for testing latest features):
   - Download [Chrome Canary](https://www.google.com/chrome/canary/)
   - Or download [Chrome Dev](https://www.google.com/chrome/dev/)

3. **Verify Fix**:
   - Enter `chrome://version/` in browser address bar to check version number
   - If version >= 145, Cookie modification should work normally

---

## Q: How to use on macOS or Linux?

**Current Status:**
Official builds are only provided for Windows, macOS and Linux platforms are theoretically supported but not fully tested.

**Solution:**
1. Refer to the "Building from Source" section in [Quick Start](./02-quick-start.md)
2. Need to install Go, Node.js and Wails CLI
3. If you encounter problems, please provide feedback through [GitHub Issues](https://github.com/241x/cdpnetool/issues)

---

## Q: How to switch language?

**Two Methods:**

1. **Quick Switch**: Click the language button (globe icon) on the top toolbar
2. **Switch in Settings**: Click Settings button → General Settings → Language selection

Interface takes effect immediately after switching, supports Simplified Chinese and English.

---

## Q: How to switch theme (dark/light mode)?

**Steps:**

1. Click the settings button (gear icon) on the top toolbar
2. Select theme in General Settings: Light, Dark, Follow System
3. Takes effect immediately after switching

> 💡 **Tip**: Follow System mode will automatically adapt to the operating system's theme settings

---

## Q: What's the difference between Network (Request Preview) and Events (Event Monitoring) panels?

**Difference Explanation:**

| Panel | Display Content | Use Case |
|------|----------|----------|
| **Events** | Only shows requests matching rules | Debug rules, view interception results |
| **Network** | Shows all network requests | Full traffic monitoring, analyze page requests |

**Usage Tips:**
- Network panel needs to manually enable "Start Capture" button
- After enabling, will capture all requests, not limited to those matching rules
- Must attach target page before enabling capture

---

## Q: How to configure custom browser path?

**Steps:**

1. Click the settings button on the top toolbar
2. Switch to "Browser Settings" tab
3. Click "Select File" button in "Browser Path" section
4. Select browser executable file (such as `chrome.exe` or `msedge.exe`)
5. Click "Save"

> 💡 **Tip**: Leave blank to automatically detect system Chrome/Edge

---

## Q: Where is data stored? How to backup?

**Storage Location:**
- Windows: `%USERPROFILE%\AppData\Roaming\cdpnetool\`
- Contains SQLite database (`data.db`) and log files

**Quick Access:**
- Click the info icon in the bottom status bar to open About page
- Click "Open Data Directory" or "Open Log Directory" button

**Backup Methods:**
1. Use "Export" function to export configuration as JSON file
2. Or directly copy database file `data.db`

---

## Q: Does it support WebSocket interception?

Current version does not support WebSocket interception, only supports HTTP/HTTPS requests (including XHR and Fetch).

---

## Q: Do I need to install HTTPS certificate?

No. cdpnetool is based on Chrome DevTools Protocol and directly controls the browser underlying, no certificate installation needed to intercept HTTPS requests.

---

## Q: How to report a bug?

1. Visit GitHub Issues: `https://github.com/241x/cdpnetool/issues`
2. Provide the following information:
   - System version (Windows 10/11)
   - Tool version
   - Reproduction steps
   **Error screenshots or log files**:
   - Logs located at `%USERPROFILE%\AppData\Roaming\cdpnetool\logs\`
   - Can quickly access through "Open Log Directory" button on About page

---

## Still Have Questions?

If the above content did not solve your problem, you can:

- 📖 Re-read [Quick Start](./02-quick-start.md) to confirm operation process
- 📚 View [Rule Configuration Reference](./03-rule-reference.md) to learn detailed syntax
- 💬 Ask questions in GitHub Issues or view existing discussions
