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
import 'package:http/http.dart' as http;
import 'dart:convert';
import 'package:web_demo/main.dart';
import 'package:web_demo/auth_service.dart';

class NewSimulationPage extends StatefulWidget {
  const NewSimulationPage({super.key});

  @override
  State<NewSimulationPage> createState() => _NewSimulationPageState();
}

class _NewSimulationPageState extends State<NewSimulationPage> {
  final _formKey = GlobalKey<FormState>();
  final _filePathController = TextEditingController();
  final _buildIdController = TextEditingController();
  final _instanceTypeController = TextEditingController();
  final _maxSimTimeController = TextEditingController();
  final _maxReportCountController = TextEditingController();
  bool _isSubmitting = false;

  @override
  void initState() {
    super.initState();
    // Initialize default form values.
    _initializeDefaultValues();
  }

  @override
  void dispose() {
    _filePathController.dispose();
    _buildIdController.dispose();
    _instanceTypeController.dispose();
    _maxSimTimeController.dispose();
    _maxReportCountController.dispose();
    super.dispose();
  }

  void _initializeDefaultValues() {
    _filePathController.text = 'manualtest1/';
    _buildIdController.text = 'latest';
    _instanceTypeController.text = 'n1-standard-8';
    _maxSimTimeController.text = '60';
    _maxReportCountController.text = '1';
  }

  Future<void> _submitForm() async {
    if (_isSubmitting) return;

    if (_formKey.currentState!.validate()) {
      setState(() => _isSubmitting = true);
      final loggedInUserEmail = authService.userEmail;

      if (loggedInUserEmail == null) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text('Could not identify user. Please sign in again.'),
          ),
        );
        setState(() => _isSubmitting = false);
        return;
      }

      final simulationData = {
        "owner": loggedInUserEmail,
        "file_path": _filePathController.text,
        "build_id": _buildIdController.text,
        "instance_type": _instanceTypeController.text,
        "max_simulation_time": int.parse(_maxSimTimeController.text),
        "max_report_count": int.parse(_maxReportCountController.text),
      };

      try {
        Map<String, String>? headers = await authService.getIDTokenHeaders();
        headers!['Content-Type'] = 'application/json';

        final url = '${configService.getBackendUrl()}/api/scheduleSimulation';

        final response = await http.post(
          Uri.parse(url),
          headers: headers,
          body: jsonEncode(simulationData),
        );

        if (!mounted) return;

        if (response.statusCode == 201 || response.statusCode == 200) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(content: Text('Simulation scheduled successfully!')),
          );
          Navigator.pop(context);
        } else {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text(
                'Failed to schedule simulation. Error: ${response.body}',
              ),
            ),
          );
        }
      } catch (e) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Error scheduling simulation: $e')),
        );
      } finally {
        if (mounted) {
          setState(() => _isSubmitting = false);
        }
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text("Schedule New Simulation")),
      body: Form(
        key: _formKey,
        child: Padding(
          padding: const EdgeInsets.all(16.0),
          child: SingleChildScrollView(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                const Text(
                  'Enter the details for your new simulation:',
                  style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
                ),
                const SizedBox(height: 20),
                TextFormField(
                  controller: _filePathController,
                  decoration: const InputDecoration(labelText: "File Path"),
                  validator: (value) => value == null || value.isEmpty
                      ? "File Path is required"
                      : null,
                ),
                const SizedBox(height: 12),
                TextFormField(
                  controller: _buildIdController,
                  decoration: const InputDecoration(labelText: "Build ID"),
                  validator: (value) => value == null || value.isEmpty
                      ? "Build ID is required"
                      : null,
                ),
                const SizedBox(height: 12),
                TextFormField(
                  controller: _instanceTypeController,
                  decoration: const InputDecoration(labelText: "Instance Type"),
                  validator: (value) => value == null || value.isEmpty
                      ? "Instance Type is required"
                      : null,
                ),
                const SizedBox(height: 12),
                TextFormField(
                  controller: _maxSimTimeController,
                  decoration: const InputDecoration(
                    labelText: "Max Simulation Time (seconds)",
                  ),
                  keyboardType: TextInputType.number,
                  validator: (value) {
                    if (value == null || value.isEmpty) {
                      return "This field is required";
                    }
                    if (int.tryParse(value) == null || int.parse(value) <= 0) {
                      return "Enter a valid positive integer";
                    }
                    return null;
                  },
                ),
                const SizedBox(height: 12),
                TextFormField(
                  controller: _maxReportCountController,
                  decoration: const InputDecoration(
                    labelText: "Max Report Count",
                  ),
                  keyboardType: TextInputType.number,
                  validator: (value) {
                    if (value == null || value.isEmpty) {
                      return "This field is required";
                    }
                    if (int.tryParse(value) == null || int.parse(value) <= 0) {
                      return "Enter a valid positive integer";
                    }
                    return null;
                  },
                ),
                const SizedBox(height: 24),
                ElevatedButton(
                  onPressed: _isSubmitting ? null : _submitForm,
                  child: _isSubmitting
                      ? const SizedBox(
                          width: 20,
                          height: 20,
                          child: CircularProgressIndicator(strokeWidth: 3),
                        )
                      : const Text("Submit"),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
