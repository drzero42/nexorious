<script lang="ts">
	import { RouteGuard } from '$lib/components';

	// Section definitions for the settings hub
	const sections = [
		{
			id: 'profile',
			name: 'Profile',
			description: 'Manage your account information and security',
			icon: '👤',
			href: '/profile'
		},
		{
			id: 'tags',
			name: 'Tags',
			description: 'Organize your games with custom tags',
			icon: '🏷️',
			href: '/tags'
		},
		{
			id: 'jobs',
			name: 'Background Jobs',
			description: 'View and manage import, sync, and export tasks',
			icon: '⚙️',
			href: '/jobs'
		}
	];
</script>

<svelte:head>
	<title>Settings - Nexorious</title>
</svelte:head>

<RouteGuard requireAuth={true}>
	<div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8 space-y-8">
		<!-- Header -->
		<div>
			<nav class="flex text-sm text-gray-500 dark:text-gray-400" aria-label="Breadcrumb">
				<ol class="inline-flex items-center space-x-1 md:space-x-3">
					<li>
						<a href="/dashboard" class="hover:text-gray-700 dark:hover:text-gray-300">Dashboard</a>
					</li>
					<li>
						<span class="mx-2">›</span>
					</li>
					<li>
						<span class="text-gray-900 dark:text-white font-medium">Settings</span>
					</li>
				</ol>
			</nav>

			<div class="mt-4">
				<h1
					class="text-2xl font-bold leading-7 text-gray-900 dark:text-white sm:truncate sm:text-3xl sm:tracking-tight"
				>
					Settings
				</h1>
				<p class="mt-2 text-sm text-gray-500 dark:text-gray-400 max-w-2xl">
					Manage your account, tags, and view background tasks.
				</p>
			</div>
		</div>

		<!-- Settings Sections Grid -->
		<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
			{#each sections as section}
				<a
					href={section.href}
					class="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 p-6 hover:border-indigo-400 dark:hover:border-indigo-600 hover:shadow-md transition-all duration-200 group"
				>
					<div class="flex items-center space-x-4">
						<div
							class="w-12 h-12 rounded-lg bg-gray-100 dark:bg-gray-700 flex items-center justify-center text-2xl group-hover:bg-indigo-100 dark:group-hover:bg-indigo-900/30 transition-colors"
						>
							{section.icon}
						</div>
						<div class="flex-1 min-w-0">
							<h3
								class="text-lg font-semibold text-gray-900 dark:text-white group-hover:text-indigo-600 dark:group-hover:text-indigo-400 transition-colors"
							>
								{section.name}
							</h3>
							<p class="text-sm text-gray-500 dark:text-gray-400">{section.description}</p>
						</div>
						<svg
							class="w-5 h-5 text-gray-400 group-hover:text-indigo-500 transition-colors"
							fill="none"
							viewBox="0 0 24 24"
							stroke="currentColor"
						>
							<path
								stroke-linecap="round"
								stroke-linejoin="round"
								stroke-width="2"
								d="M9 5l7 7-7 7"
							/>
						</svg>
					</div>
				</a>
			{/each}
		</div>

		<!-- Quick Actions -->
		<section>
			<h2 class="text-lg font-semibold text-gray-900 dark:text-white mb-4">Quick Actions</h2>
			<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
				<a
					href="/profile"
					class="flex items-center p-4 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors"
				>
					<svg
						class="w-5 h-5 text-gray-400 mr-3"
						fill="none"
						viewBox="0 0 24 24"
						stroke="currentColor"
					>
						<path
							stroke-linecap="round"
							stroke-linejoin="round"
							stroke-width="2"
							d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"
						/>
					</svg>
					<span class="text-sm font-medium text-gray-700 dark:text-gray-300">Change Username</span>
				</a>

				<a
					href="/profile"
					class="flex items-center p-4 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors"
				>
					<svg
						class="w-5 h-5 text-gray-400 mr-3"
						fill="none"
						viewBox="0 0 24 24"
						stroke="currentColor"
					>
						<path
							stroke-linecap="round"
							stroke-linejoin="round"
							stroke-width="2"
							d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"
						/>
					</svg>
					<span class="text-sm font-medium text-gray-700 dark:text-gray-300">Change Password</span>
				</a>

				<a
					href="/tags"
					class="flex items-center p-4 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors"
				>
					<svg
						class="w-5 h-5 text-gray-400 mr-3"
						fill="none"
						viewBox="0 0 24 24"
						stroke="currentColor"
					>
						<path
							stroke-linecap="round"
							stroke-linejoin="round"
							stroke-width="2"
							d="M12 6v6m0 0v6m0-6h6m-6 0H6"
						/>
					</svg>
					<span class="text-sm font-medium text-gray-700 dark:text-gray-300">Create New Tag</span>
				</a>

				<a
					href="/jobs"
					class="flex items-center p-4 bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 hover:bg-gray-50 dark:hover:bg-gray-700/50 transition-colors"
				>
					<svg
						class="w-5 h-5 text-gray-400 mr-3"
						fill="none"
						viewBox="0 0 24 24"
						stroke="currentColor"
					>
						<path
							stroke-linecap="round"
							stroke-linejoin="round"
							stroke-width="2"
							d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"
						/>
					</svg>
					<span class="text-sm font-medium text-gray-700 dark:text-gray-300">View All Jobs</span>
				</a>
			</div>
		</section>

		<!-- Related Links -->
		<div class="flex flex-wrap gap-4 text-sm pt-4 border-t border-gray-200 dark:border-gray-700">
			<a
				href="/sync"
				class="text-indigo-600 dark:text-indigo-400 hover:text-indigo-800 dark:hover:text-indigo-300 inline-flex items-center"
			>
				<svg class="h-4 w-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
					/>
				</svg>
				Sync Settings
			</a>
			<a
				href="/import-export"
				class="text-indigo-600 dark:text-indigo-400 hover:text-indigo-800 dark:hover:text-indigo-300 inline-flex items-center"
			>
				<svg class="h-4 w-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12"
					/>
				</svg>
				Import / Export
			</a>
			<a
				href="/games"
				class="text-indigo-600 dark:text-indigo-400 hover:text-indigo-800 dark:hover:text-indigo-300 inline-flex items-center"
			>
				<svg class="h-4 w-4 mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
					<path
						stroke-linecap="round"
						stroke-linejoin="round"
						stroke-width="2"
						d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"
					/>
				</svg>
				View Collection
			</a>
		</div>
	</div>
</RouteGuard>
