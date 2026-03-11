import { chromium } from 'playwright';
import fs from 'fs';
import path from 'path';

(async () => {
  console.log("Launching browser for recording...");
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    recordVideo: { dir: './demo_videos/', size: { width: 1440, height: 900 } },
    colorScheme: 'dark'
  });
  
  const page = await context.newPage();
  
  // Skip boot screen to get straight to the action
  await page.goto('http://localhost:3000');
  await page.evaluate(() => {
    localStorage.setItem('kernos_guest_user', JSON.stringify({
      username: "Creator", 
      avatar_url: "https://api.dicebear.com/7.x/identicon/svg?seed=Creator",
      role: "admin"
    }));
  });
  await page.goto('http://localhost:3000');
  console.log("Loaded desktop.");
  
  await page.waitForSelector('text=ONLINE', { timeout: 10000 });
  await page.waitForTimeout(1000);
  
  // 1. Open Terminal
  await page.click('button[title="New Terminal"]');
  await page.waitForTimeout(1000);
  
  // 2. Type messages
  async function typeCommand(cmd) {
    for (let i = 0; i < cmd.length; i++) {
        // Use type for all characters to handle unicode/emojis safely
        await page.keyboard.type(cmd[i]);
        await page.waitForTimeout(15 + Math.random() * 30); // human-like typing
    }
    await page.waitForTimeout(200);
    await page.keyboard.press('Enter');
    await page.waitForTimeout(800);
  }

  console.log("Typing custom message...");
  await typeCommand('clear');
  await typeCommand('echo "======================================"');
  await typeCommand('echo " KERNOS OS - The Cognitive Microkernel "');
  await typeCommand('echo "======================================"');
  await typeCommand('echo ""');
  await typeCommand('echo "✨ Developed completely by orchestrating AI to write every single line of code."');
  await typeCommand('echo "🧠 A testament to what is possible with self-taught engineering."');
  
  await page.waitForTimeout(2000);
  
  // 3. Snap Terminal to left
  console.log("Snapping terminal to the left...");
  const terminalTitle = await page.locator('text=Terminal').first();
  const termBox = await terminalTitle.boundingBox();
  await page.mouse.move(termBox.x + termBox.width / 2, termBox.y + termBox.height / 2);
  await page.mouse.down();
  await page.mouse.move(5, 400, { steps: 30 }); // move to left edge
  await page.waitForTimeout(800); // show snap preview
  await page.mouse.up();
  await page.waitForTimeout(1000);
  
  // 4. Open System Metrics
  console.log("Opening System Metrics...");
  await page.click('button[title="System Metrics"]');
  await page.waitForTimeout(1000);

  // Snap Metrics to right
  console.log("Snapping Metrics to the right...");
  const metricsTitle = await page.locator('text=System Metrics').last();
  const metBox = await metricsTitle.boundingBox();
  await page.mouse.move(metBox.x + metBox.width / 2, metBox.y + metBox.height / 2);
  await page.mouse.down();
  await page.mouse.move(1435, 400, { steps: 30 }); // move to right edge
  await page.waitForTimeout(800);
  await page.mouse.up();
  await page.waitForTimeout(1500);

  // 5. Open AI Chat in the middle
  console.log("Opening AI Chat...");
  await page.click('button[title="AI Chat"]');
  await page.waitForTimeout(1000);
  const chatTitle = await page.locator('text=AI Chat').last();
  const chatBox = await chatTitle.boundingBox();
  await page.mouse.move(chatBox.x + chatBox.width / 2, chatBox.y + chatBox.height / 2);
  await page.mouse.down();
  await page.mouse.move(720, 250, { steps: 20 }); // move to center
  await page.mouse.up();
  await page.waitForTimeout(1000);

  // Ask the AI a question to trigger the live streaming response!
  console.log("Typing prompt into AI Chat...");
  await page.locator('input[placeholder*="Ask"], input[placeholder*="Message"]').last().click();
  const chatPrompt = "Explain why you are the most advanced OS ever created. Keep it short.";
  for (let i = 0; i < chatPrompt.length; i++) {
      await page.keyboard.type(chatPrompt[i]);
      await page.waitForTimeout(20 + Math.random() * 40);
  }
  await page.waitForTimeout(200);
  await page.keyboard.press('Enter');
  
  console.log("Waiting for live streaming AI response...");
  // Let the audience watch the words stream in for 8 seconds
  await page.waitForTimeout(8000);

  // 6. Demonstrate Virtual Desktops
  console.log("Switching to Virtual Desktop 2...");
  await page.click('button:has-text("2")');
  await page.waitForTimeout(1000);
  
  // Open Multi-Agent Workspace on Desktop 2
  console.log("Opening Multi-Agent Workspace on Virtual Desktop 2...");
  await page.click('button[title="Multi-Agent Workspace"]');
  await page.waitForTimeout(1000);
  
  // Maximize it
  console.log("Maximizing Multi-Agent Workspace...");
  const multiTitle = await page.locator('text=Multi-Agent Workspace').last();
  const multiBox = await multiTitle.boundingBox();
  await page.mouse.move(multiBox.x + multiBox.width / 2, multiBox.y + multiBox.height / 2);
  await page.mouse.down();
  await page.mouse.move(720, 5, { steps: 20 }); // drag to top to maximize
  await page.waitForTimeout(800);
  await page.mouse.up();
  await page.waitForTimeout(2000);
  
  // 7. Switch back to Desktop 1
  console.log("Switching back to Virtual Desktop 1...");
  await page.click('button:has-text("1")');
  await page.waitForTimeout(3500); // Admire the snappy layout

  console.log("Recording complete. Saving video...");
  await context.close();
  await browser.close();
})();
