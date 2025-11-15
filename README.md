# Quatplot - Real-time 3D Quaternion Visualizer

A Go application that reads quaternion data from a serial port and visualizes it by rotating a 3D model in real-time through a web interface using WebGL/Three.js.

## Features

- Reads quaternion data from serial port in real-time
- Real-time 3D model rotation based on quaternion input
- Web-based GUI with WebGL rendering
- Load custom .OBJ 3D models
- Reset orientation to default
- Auto-reconnection for serial port
- WebSocket for low-latency data streaming

## Prerequisites

- Go
- A serial device sending quaternion data in the format: `i,j,k,real` (one per line)

## Usage

### Basic Usage

Run with default settings (COM3, 115200 baud, web server on port 8080):

```
go run main.go
```

### Command Line Options

```
go run main.go -port COM3 -baud 115200 -web 8080
```

**Available flags:**
- `-port` : Serial port name (default: "COM3")
  - Windows: COM1, COM3, COM4, etc.
  - Linux: /dev/ttyUSB0, /dev/ttyACM0, etc.
  - macOS: /dev/cu.usbserial-*, /dev/cu.usbmodem*
- `-baud` : Baud rate (default: 115200)
- `-web` : HTTP server port (default: "8080")

### Examples

**Windows:**
```
go run main.go -port COM4 -baud 9600
```

**Linux/macOS:**
```
go run main.go -port /dev/ttyUSB0 -baud 115200
```

**Custom web port:**
```
go run main.go -port COM3 -web 3000
```

## Web Interface

1. Open your browser and navigate to: `http://localhost:8080`
2. The interface shows:
   - **Load Model Files** button: Upload 3D model files (.obj and optionally .mtl)
   - **Reset Orientation** button: Reset the model to default orientation
   - **Reset Zoom** button: Reset camera zoom to default distance
   - **Connection Status**: Shows WebSocket connection state
   - **Quaternion Data**: Real-time display of i, j, k, real values
   - **Model Info**: Shows currently loaded model name(s)
   - **Zoom Info**: Shows current camera distance

### Controls

- **Mouse Wheel**: Zoom in and out (scroll up to zoom in, scroll down to zoom out)
- **Click + Drag**: Manually rotate the object - sensor quaternion is applied relatively on top of manual rotation
- **Load Model Files**: Click to upload 3D model files
  - Select a single .obj file for a model without materials
  - Select both .obj and .mtl files (multi-select or drag-and-drop) to load with materials and textures
  - The .mtl file defines materials, colors, and texture properties
- **Reset Orientation**: Return both manual and sensor quaternion to identity (no rotation)
- **Reset Zoom**: Return camera to default distance (5.0)

### Rotation Behavior

The application supports **relative quaternion rotation**, meaning:
- You can manually rotate the object with your mouse (click and drag)
- The incoming sensor quaternion data is applied **on top of** your manual rotation
- This allows you to view the sensor's orientation changes from any angle you choose
- Reset Orientation clears both manual and sensor rotations

### Default Behavior

- Without loading a model, a colored cube with visible edges is displayed
- The model rotates in real-time based on incoming quaternion data
- All models are automatically centered and scaled to fit the viewport
- Zoom range: distance 0.1 (very close) to 50.0 (far)
- Cursor changes to grab/grabbing icon when rotating

### Loading 3D Models

**OBJ Files Only:**
- Select a single .obj file
- A default blue material will be applied

**OBJ + MTL Files:**
- Select both .obj and .mtl files (hold Ctrl/Cmd to multi-select, or drag both files)
- Materials, colors, and properties from the .mtl file will be applied
- If texture references exist in the .mtl file, they won't be loaded (file paths only, no image loading)

## Input Data Format

The serial port should send quaternion data as comma-separated values, one quaternion per line:

```
i,j,k,real
```

**Example:**
```
0.0,0.0,0.0,1.0
0.1,0.2,0.3,0.9
-0.5,0.3,0.1,0.8
```

Where:
- `i` = x-component of quaternion
- `j` = y-component of quaternion
- `k` = z-component of quaternion
- `real` = w-component (scalar part) of quaternion

## Architecture

### Backend (Go)
- Reads from serial port continuously
- Parses quaternion data (i,j,k,real format)
- Broadcasts data to all connected WebSocket clients
- Serves embedded HTML/JavaScript frontend
- Auto-reconnects to serial port on disconnect

### Frontend (JavaScript/Three.js)
- Establishes WebSocket connection to backend
- Renders 3D scene with WebGL
- Applies quaternion rotations to loaded model
- Handles .OBJ file loading and parsing
- Provides user controls for model management

## License

This project is provided as-is for educational and development purposes.

## Contributing

Feel free to submit issues and pull requests!
