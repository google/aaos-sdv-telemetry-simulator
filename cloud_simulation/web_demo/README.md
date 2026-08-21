
# WEB DEMO: Flutter Web App with Google Authentication, Firestore Access, and Simulation Management

This Flutter web app allows users to sign in using **Google Identity Services**, interact with **Firestore directly via HTTP requests**, and manage simulations (e.g., scheduling and deletion). The application is hosted on **Google App Engine**, with two services:

1. **Flutter as the main service** (running via `python gunicorn`) handling authentication and authorization, listing simulations from the firestore database and allows to send requests for new simulations or cancellations.
2. **Backend service** (implemented with Python) invoking Cloud Functions like scheduling and deleting simulations.

This document explains how the app is set up, hosted, and important configurations needed to replicate the project.

---

## Features

- **Google Authentication**:
  - Implements **Google Identity Services (GIS)** using the `google_sign_in_web` plugin.
  - Supports separating **authentication** (signing in users) and **authorization** (requesting additional scopes).
  - No user management required: Permissions are taken from GCP's IAM.

- **Firestore HTTP Integration**:
  - Direct API integration with Firestore via long-polling and HTTP requests.
  - No `firebase` dependency; the app uses Google's REST APIs.

- **Simulation Management**:
  - Supports scheduling and deleting simulations via HTTP calls to a **backend Python service**.
  - Manages simulations with authenticated API calls to Firestore.

- **Hosted on Google App Engine**:
  - Flutter serves as the main web app under the `default` App Engine service.
  - Python backend serves API endpoints as a secondary service under App Engine.

- **Web-Only Platform**:
  - The app is built exclusively for the **web platform**, with no support for other platforms like iOS or Android due to GIS usage.

---

## Prerequisites

To run or deploy this application, you’ll need the following:

### 1. **Flutter SDK**

- Install Flutter ([official guide](https://docs.flutter.dev/get-started/install)).
- Ensure `flutter doctor` runs with no issues. The app targets the **web platform** only:

     ```bash
     flutter config --enable-web
     flutter devices
     ```

### 2. **Google Cloud Project**

- Enable the following APIs:
  - **Firestore API**
  - **Google Identity Services API**

- **Create OAuth 2.0 Credentials** for Web:
  - Go to **APIs & Services > Credentials > Create Credentials > OAuth 2.0 Client IDs**.
  - Set the following:
    - **Application Type**: Web Application
    - **Authorized JavaScript Origins**:

         ```
         http://localhost:5000
         https://YOUR_PROJECT_ID.appspot.com
         ```

    - **Authorized Redirect URIs**:

         ```
         http://localhost:5000
         https://YOUR_PROJECT_ID.appspot.com
         ```

  - Save the **Client ID** and use it in the app setup.

## Folder Structure

```
backend/
├── app.yaml                          # App Engine configuration for backend service
├── main.py                           # Handles HTTP requests for scheduling/deletion and manages CORS
└── requirements.txt                  # Backend dependencies
frontend/
├── lib/
│   ├── main.dart                     # Entry point for Flutter app
│   ├── auth_service.dart             # Manages authentication & scope authorization
│   ├── auth_wrapper.dart             # Handles user login and app lifecycle
│   ├── simulation_list_page.dart     # Manages Firestore HTTP polling for simulations
│   └── new_simulation_page.dart      # UI to schedule a new simulation
├── web/
│   ├── index.html                    # HTML file for Flutter web platform
│   └── other assets...               # Static assets
├── pubspec.yaml                      # Flutter project dependencies
├── app.yaml                          # App Engine configuration for Flutter service
├── main.py                           # Handles serving flutter html content with flask
├── requirements.txt                  # frontend dependencies
├── backend-app.yaml                  # App Engine configuration for Python backend service
└── README.md                         # Documentation for the project
```

---

## Configuration

### **1. App Engine Configuration**

You have **two App Engine services**: the `default` service that runs the Flutter frontend and handles Firestore interactions and the `backend` service that invokes the Cloud Simulator Cloud Functions.

#### `app.yaml` (Main Flutter Service Configuration)

```yaml
runtime: python39
entrypoint: gunicorn -b :$PORT main:app

handlers:
  - url: /.*
    static_dir: build/web
```

#### `backend-app.yaml` (Python Backend Service Configuration)

```yaml
runtime: python39
entrypoint: python3 app.py

handlers:
  - url: /.* # Direct API requests to this service.
    script: auto
```

### **2. Backend Python Service (Firestore Requests)**

This service interacts directly with Firestore (e.g., to schedule or delete simulations). Example Python code for interacting with Firestore:

**`app.py`:**

```python
from flask import Flask, request, jsonify
import requests

app = Flask(__name__)

# Firestore Configuration
FIRESTORE_PROJECT = "YOUR_PROJECT_ID"
FIRESTORE_API_URL = f"https://firestore.googleapis.com/v1/projects/{FIRESTORE_PROJECT}/databases/<...>/documents/records"
AUTH_HEADER = {"Authorization": "Bearer YOUR_OAUTH2_TOKEN"}  # Replace with the token-fetching logic

@app.route("/api/scheduleSimulation", methods=["POST"])
def schedule_simulation():
    """Schedule a new simulation (adds a record to Firestore)."""
    simulation_data = request.json
    response = requests.post(FIRESTORE_API_URL, json=simulation_data, headers=AUTH_HEADER)
    return jsonify(response.json()), response.status_code

@app.route("/api/deleteSimulation/<simulation_id>", methods=["DELETE"])
def delete_simulation(simulation_id):
    """Delete a simulation from Firestore."""
    url = f"{FIRESTORE_API_URL}/{simulation_id}"
    response = requests.delete(url, headers=AUTH_HEADER)
    return jsonify(response.json()), response.status_code

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8080)
```

**`requirements.txt`:**

```txt
Flask==2.1.2
requests==2.27.1
gunicorn==20.1.0
```

---

## Hosting on Google App Engine

### 1. Build the Flutter Web App

```bash
cd frontend && flutter build web
```

### 2. Deploy Flutter Frontend

```bash
cd frontend && gcloud app deploy app.yaml
```

### 3. Deploy Python Backend

```bash
cd backend && gcloud app deploy app.yaml
```

### 4. Access Deployed Services

- **Main Flutter Web App**:

  ```
  https://YOUR_PROJECT_ID.appspot.com
  ```

- **Backend APIs** (e.g., `scheduleSimulation`):

  ```
  https://backend-dot-YOUR_PROJECT_ID.appspot.com/api/scheduleSimulation
  ```

---

## Important Notes

1. **Authentication and Authorization Flow:**
   - Authentication and authorization are separate:
     - Authentication happens during `GoogleSignIn().signIn()`.
     - Authorization (scopes approval) happens with `GoogleSignIn().requestScopes()`.

2. **Firestore API Integration:**
   - Firestore is accessed directly via HTTP requests. Ensure the API keys and OAuth tokens are properly secured.
   - Use the `Authorization: Bearer <token>` header in every request.

3. **OAuth Configuration:**
   - Ensure your OAuth Client ID is configured for both localhost and production domains.

4. **Long-Polling for Firestore:**
   - The app fetches Firestore records via HTTP and refreshes automatically.

5. **Web-Only Platform:**
   - This app supports **web** only, and no mobile targets are configured.

---

## Useful Commands

- **Run Locally**:

  ```bash
  flutter run -d web
  ```

- **Build for Web**:

  ```bash
  flutter build web
  ```

- **Deploy to App Engine**:

  ```bash
  gcloud app deploy
  ```

---

## Troubleshooting

- **401 Unauthorized on Firestore**:
  - Verify that tokens include all required scopes for Firestore access.
  - Ensure Firestore permissions match your security rules.

- **Cross-Origin Errors**:
  - Ensure OAuth Client `Authorized JavaScript Origins` are properly configured in the Google Cloud Console.
