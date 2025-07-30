package org.bu1ld.task

data class Task(
  val name: String,
  val description: String = "",
  val dependsOn: List<String> = emptyList(),
  val action: suspend () -> Unit
)

object TaskRegistry {
  private val tasks = mutableMapOf<String, Task>()

  fun register(task: Task) {
    if (task.name in tasks) error("Task '${task.name}' already registered")
    tasks[task.name] = task
  }

  fun get(name: String): Task = tasks[name] ?: error("Task '$name' not found")

  fun all(): Collection<Task> = tasks.values
}