// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import 'dart:convert';

import 'package:flutter/services.dart' show rootBundle;

class ConfigService {
  // Private variable to hold the loaded configuration
  Map<String, dynamic>? _config;

  /// Loads the configuration file from assets.
  Future<void> loadConfig() async {
    final configString = await rootBundle.loadString('assets/config.json');
    _config = json.decode(configString) as Map<String, dynamic>;
  }

  /// Getter for the Web Client ID.
  String getWebClientId() {
    if (_config == null) {
      throw Exception("Config not loaded. Call loadConfig() first.");
    }
    final String? clientId = _config!['webClientId'] as String?;
    if (clientId == null) {
      throw Exception("webClientId not found in config.json.");
    }
    return clientId;
  }

  String getFirestoreApiBaseUrl() {
    if (_config == null) {
      throw Exception("Config not loaded. Call loadConfig() first.");
    }
    final String? firestoreApiBaseUrl =
        _config!['firestoreApiBaseUrl'] as String?;
    if (firestoreApiBaseUrl == null) {
      throw Exception("firestoreApiBaseUrl not found in config.json.");
    }
    return firestoreApiBaseUrl;
  }

  String getBackendUrl() {
    if (_config == null) {
      throw Exception("Config not loaded. Call loadConfig() first.");
    }
    final String? backendUrl = _config!['backendUrl'] as String?;
    if (backendUrl == null) {
      throw Exception("backendUrl not found in config.json.");
    }
    return backendUrl;
  }

  String getBucketName() {
    if (_config == null) {
      throw Exception("Config not loaded. Call loadConfig() first.");
    }
    final String? bucketName = _config!['bucketName'] as String?;
    if (bucketName == null) {
      throw Exception("bucketName not found in config.json.");
    }
    return bucketName;
  }
}
