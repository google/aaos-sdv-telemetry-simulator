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

import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;
import 'package:web_demo/main.dart';
import 'package:web_demo/auth_service.dart';

class SimulationDetailPage extends StatefulWidget {
  final String simulationId;
  final Map<String, dynamic> simulationData;

  const SimulationDetailPage({
    super.key,
    required this.simulationId,
    required this.simulationData,
  });

  @override
  _SimulationDetailPageState createState() => _SimulationDetailPageState();
}

class _SimulationDetailPageState extends State<SimulationDetailPage> {
  // State variables
  Map<String, dynamic> _currentSimulationData = {};
  List<String> outputFiles = [];
  String? selectedFileContent;
  String? selectedFileName;
  bool _isLoadingFiles = true;
  bool _isLoadingFileContent = false;
  bool _isActionInProgress = false;
  String? _errorMessage;

  @override
  void initState() {
    super.initState();
    _currentSimulationData = Map.from(widget.simulationData);
    _fetchOutputFiles();
  }

  Future<void> _fetchOutputFiles() async {
    setState(() {
      _isLoadingFiles = true;
      _errorMessage = null;
    });

    try {
      final String bucketName = configService.getBucketName();
      final String folderPath =
          'simulations/${widget.simulationId}/outputs/telemetry_simulator_out/';
      final Uri url = Uri.parse(
        'https://storage.googleapis.com/storage/v1/b/$bucketName/o?prefix=$folderPath',
      );

      final headers = await authService.getAuthHeaders();

      final response = await http.get(url, headers: headers);
      if (!mounted) return;

      if (response.statusCode == 200) {
        final data = jsonDecode(response.body);
        final List<String> files = (data['items'] as List? ?? [])
            .map((item) => item['name'] as String)
            .where(
              (name) =>
                  name.endsWith('.txt') && !name.contains('outputs/reports/'),
            )
            .toList();
        setState(() {
          outputFiles = files;
          if (files.isNotEmpty) {
            _fetchFileContent(files.first);
          }
        });
      } else {
        setState(
          () => _errorMessage =
              'Failed to fetch output files (HTTP ${response.statusCode}).',
        );
      }
    } catch (e) {
      setState(() => _errorMessage = 'Error fetching output files: $e.');
    } finally {
      if (mounted) {
        setState(() => _isLoadingFiles = false);
      }
    }
  }

  Future<void> _fetchFileContent(String filePath) async {
    setState(() {
      _isLoadingFileContent = true;
      selectedFileName = filePath;
      selectedFileContent = null;
      _errorMessage = null;
    });

    try {
      final headers = await authService.getIDTokenHeaders();
      headers!['Content-Type'] = 'application/json';
      final url = '${configService.getBackendUrl()}/api/getFileContent';
      final response = await http.post(
        Uri.parse(url),
        headers: headers,
        body: jsonEncode({"filePath": filePath}),
      );
      if (!mounted) return;
      if (response.statusCode == 200) {
        setState(() => selectedFileContent = response.body);
      } else {
        setState(
          () => _errorMessage =
              'Failed to fetch file content (HTTP ${response.statusCode}).',
        );
      }
    } catch (e) {
      setState(() => _errorMessage = 'Error fetching file content: $e.');
    } finally {
      if (mounted) {
        setState(() => _isLoadingFileContent = false);
      }
    }
  }

  Future<void> _cancelSimulation() async {
    setState(() {
      _isActionInProgress = true;
      _errorMessage = null;
    });
    try {
      final headers = await authService.getIDTokenHeaders();
      headers!['Content-Type'] = 'application/json';
      final url = '${configService.getBackendUrl()}/api/cancelSimulation';
      final response = await http.post(
        Uri.parse(url),
        headers: headers,
        body: jsonEncode({"id": widget.simulationId}),
      );
      if (!mounted) return;
      if (response.statusCode == 200) {
        setState(() {
          _currentSimulationData['status'] = 'cancelled';
          _currentSimulationData['status_updated_at'] = DateTime.now()
              .toUtc()
              .toIso8601String();
        });
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('Simulation cancelled successfully!')),
        );
      } else {
        setState(
          () => _errorMessage =
              'Failed to cancel simulation (HTTP ${response.statusCode}).',
        );
      }
    } catch (e) {
      setState(() => _errorMessage = 'Error cancelling simulation: $e.');
    } finally {
      if (mounted) {
        setState(() => _isActionInProgress = false);
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final simulationStatus =
        _currentSimulationData['status']?.toString().toLowerCase() ?? 'unknown';

    return Scaffold(
      appBar: AppBar(title: Text('Simulation: ${widget.simulationId}')),
      body: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            SelectableText(
              'Simulation ID: ${widget.simulationId}',
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
            ),
            const SizedBox(height: 16),
            SelectableText(
              'Status: ${_currentSimulationData['status'] ?? 'N/A'}',
            ),
            SelectableText(
              'Received At: ${_currentSimulationData['received_at'] ?? 'N/A'}',
            ),
            SelectableText(
              'Status Updated At: ${_currentSimulationData['status_updated_at'] ?? 'N/A'}',
            ),

            if (simulationStatus != 'completed' &&
                simulationStatus != 'cancelled' &&
                simulationStatus != 'failed')
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 16.0),
                child: _isActionInProgress
                    ? const CircularProgressIndicator()
                    : ElevatedButton.icon(
                        icon: const Icon(Icons.cancel),
                        label: const Text('Cancel Simulation'),
                        style: ElevatedButton.styleFrom(
                          backgroundColor: Colors.orange,
                        ),
                        onPressed: _cancelSimulation,
                      ),
              ),

            const SizedBox(height: 24),
            const Divider(),
            const SizedBox(height: 16),

            if (_errorMessage != null)
              Padding(
                padding: const EdgeInsets.only(bottom: 16.0),
                child: Text(
                  _errorMessage!,
                  style: const TextStyle(color: Colors.red),
                ),
              ),

            if (simulationStatus == 'completed')
              Expanded(child: _buildFileViewer()),
          ],
        ),
      ),
    );
  }

  Widget _buildFileViewer() {
    if (_isLoadingFiles) {
      return const Center(child: CircularProgressIndicator());
    }
    if (outputFiles.isEmpty) {
      return const Center(child: Text('No output files found.'));
    }

    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // File List
        Expanded(
          flex: 1,
          child: ListView.builder(
            itemCount: outputFiles.length,
            itemBuilder: (context, index) {
              final fileName = outputFiles[index];
              final shortName = fileName.split('/').last;
              return ListTile(
                title: Text(
                  shortName,
                  style: TextStyle(
                    fontWeight: fileName == selectedFileName
                        ? FontWeight.bold
                        : FontWeight.normal,
                  ),
                ),
                onTap: () => _fetchFileContent(fileName),
                selected: fileName == selectedFileName,
              );
            },
          ),
        ),
        const VerticalDivider(),
        // File Content Viewer
        Expanded(
          flex: 3,
          child: _isLoadingFileContent
              ? const Center(child: CircularProgressIndicator())
              : selectedFileContent != null
              ? SingleChildScrollView(
                  child: SelectableText(
                    selectedFileContent!,
                    style: const TextStyle(fontFamily: 'monospace'),
                  ),
                )
              : const Center(child: Text('Select a file to view its content.')),
        ),
      ],
    );
  }
}
