/**
 * Dynamic View Transitions for Exercise Navigation
 *
 * Implements smooth view transitions when navigating between workout list
 * and exercise detail pages by dynamically setting view-transition-name
 * based on the navigation context.
 */

// Helper function to check if we're on a specific page type
function isWorkoutListPage(url) {
  const path = new URL(url).pathname;
  return /^\/workouts\/\d{4}-\d{2}-\d{2}$/.test(path);
}

function isExerciseDetailPage(url) {
  const path = new URL(url).pathname;
  return /^\/workouts\/\d{4}-\d{2}-\d{2}\/exercises\/[^/]+$/.test(path);
}

function isExerciseInfoPage(url) {
  const path = new URL(url).pathname;
  return /^\/workouts\/\d{4}-\d{2}-\d{2}\/exercises\/[^/]+\/info$/.test(path);
}

function extractExerciseId(url) {
  const path = new URL(url).pathname;
  const match = path.match(/\/exercises\/([^/]+)/);
  return match ? match[1] : null;
}

// OLD PAGE LOGIC - Set view-transition-name when navigating away
window.addEventListener('pageswap', async (e) => {
  if (!e.viewTransition) return;

  const targetUrl = new URL(e.activation.entry.url);

  // Navigating from workout list to exercise detail/info page
  if (isWorkoutListPage(window.location.href) &&
      (isExerciseDetailPage(targetUrl.href) || isExerciseInfoPage(targetUrl.href))) {

    const exerciseId = extractExerciseId(targetUrl);
    if (!exerciseId) return;

    // Find the exercise link that was clicked
    const exerciseLink = document.querySelector(`a[data-exercise-id="${exerciseId}"]`);
    if (!exerciseLink) return;

    // Set view-transition-name on the exercise title in the list
    exerciseLink.style.viewTransitionName = `exercise-title-${exerciseId}`;

    // Remove view-transition-name after snapshots have been taken
    // (this is important for BFCache)
    await e.viewTransition.finished;
    exerciseLink.style.viewTransitionName = 'none';
  }
});

// NEW PAGE LOGIC - Set view-transition-name when arriving at page
window.addEventListener('pagereveal', async (e) => {
  if (!e.viewTransition) return;

  const currentURL = new URL(navigation.activation.entry.url);

  // Check if we have a previous URL (might not exist on initial load)
  if (!navigation.activation.from) return;

  const fromURL = new URL(navigation.activation.from.url);

  // Navigating from exercise detail/info page back to workout list
  if ((isExerciseDetailPage(fromURL.href) || isExerciseInfoPage(fromURL.href)) &&
      isWorkoutListPage(currentURL.href)) {

    const exerciseId = extractExerciseId(fromURL);
    if (!exerciseId) return;

    // Find the exercise link in the list
    const exerciseLink = document.querySelector(`a[data-exercise-id="${exerciseId}"]`);
    if (!exerciseLink) return;

    // Set view-transition-name on the exercise link
    exerciseLink.style.viewTransitionName = `exercise-title-${exerciseId}`;

    // Remove names after snapshots have been taken
    // so that we're ready for the next navigation
    await e.viewTransition.ready;
    exerciseLink.style.viewTransitionName = 'none';
  }
});
