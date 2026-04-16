// Copyright 2025 Google LLC
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

import 'package:flutter/material.dart';
import 'package:web_demo/config_service.dart';
import 'package:web_demo/auth_service.dart';
import 'package:web_demo/auth_wrapper.dart';
import 'simulation_list_page.dart';

final ConfigService configService = ConfigService();
final AuthService authService = AuthService();

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await configService.loadConfig();
  final String webClientId = configService.getWebClientId();
  await authService.init(clientId: webClientId);
  runApp(const MyApp());
}

class MyApp extends StatelessWidget {
  const MyApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Simulation Manager',
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(seedColor: Colors.deepPurple),
      ),
      // Define routes in your app
      routes: {
        '/': (context) => const AuthWrapper(), // Authentication state handler
        '/simulations':
            (context) =>
                const SimulationListPage(), // Main simulations dashboard
        // Additional routes can be added here
      },
    );
  }
}
