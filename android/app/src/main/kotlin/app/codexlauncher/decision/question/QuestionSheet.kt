package app.codexlauncher.decision.question

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.FilterChip
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import app.codexlauncher.decision.approval.DecisionRequest

@Composable
fun QuestionSheet(
    request: DecisionRequest,
    sending: Boolean,
    onSubmit: (Map<String, List<String>>) -> Unit,
    onNotNow: () -> Unit,
) {
    val answers = remember(request.requestId) { mutableStateMapOf<String, String>() }
    val secret = request.questions.any { it.secret }
    AlertDialog(
        onDismissRequest = onNotNow,
        title = { Text("Codex needs your answer") },
        text = {
            Column(
                modifier = Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                Text("${request.computerName} · ${request.projectLabel}")
                if (secret) {
                    QuestionFallback()
                } else {
                    request.questions.forEach { question ->
                        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            Text(question.header)
                            Text(question.prompt)
                            if (question.options.isEmpty()) {
                                OutlinedTextField(
                                    value = answers[question.id].orEmpty(),
                                    onValueChange = { answers[question.id] = it },
                                    modifier =
                                        Modifier
                                            .fillMaxWidth()
                                            .semantics { contentDescription = "Answer: ${question.header}" },
                                    singleLine = false,
                                )
                            } else {
                                question.options.forEach { option ->
                                    FilterChip(
                                        selected = answers[question.id] == option,
                                        onClick = { answers[question.id] = option },
                                        label = { Text(option) },
                                    )
                                }
                            }
                        }
                    }
                }
            }
        },
        confirmButton = {
            if (!secret) {
                Button(
                    enabled = !sending && request.questions.all { !answers[it.id].isNullOrBlank() },
                    onClick = { onSubmit(answers.mapValues { listOf(it.value) }) },
                ) { Text("Send answer") }
            }
        },
        dismissButton = {
            TextButton(enabled = !sending, onClick = onNotNow) { Text(if (secret) "Answer on computer" else "Not now") }
        },
    )
}
