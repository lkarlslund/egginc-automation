# Egg Inc Automation

I wanted to learn how to use GoCV, and had earlier been playing Egg Inc on my Android phone. As my son started playing it, I tried it again, but this time with the goal of learning something new at the same time.

Is it cheating? No, it's optimization of your time.

## Requirements:
- LD Player 9 Android emulator for Windows (code also supports Bluestacks)
- Egg Inc installed on emulator (download XAPX and install it from Windows - https://apkcombo.com/egg-inc/com.auxbrain.egginc/download/apk)
- LOTS of CPU. I use 20 cores on my machine.

## Settings in BlueStacks:
- 1080x1920 resolution (important!)
- Hotkeys:
  - Home: HOME
  - Back: PGUP
  - Rotate: PGDN
  - Recent Apps: END

## Features:
- Rotate screen to portrait mode if needed (Bluestacks)
- Starts Egg Inc from launcher
- Watch ad for 2X boost when needed
- Watches ads for golden eggs, boosts, chicken boxes - but not for money (you can change this in the code)
- Doesn't watch ads when you have soul mirror running
- Detects and takes down drones (uses drag-clicking to prevent popups if it misses, screen shakes a bit)
- Moves game location to best spot for spotting drones
- Debug window for detection debugging
- Detect and fix "blur" bug
- Multi-threaded and likes to eat your CPU
- Disables when BlueStacks has focus, so you can do manual stuff

## Improvements that can be made:
- Launch BlueStacks if needed
- Restart BlueStacks in case of trouble
- Handle shutdown of BlueStacks more gracefully
- Auto research could be added
- Forward alerts of running out of chicken coop space or transporation limit hit
- Pyramid detection of templates to improve performance
- Logging with timestamps
- Better prediction of drone locations
- Some sort of GUI with on-the-fly settings changes
