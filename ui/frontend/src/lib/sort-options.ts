export type SortField =
  | 'title'
  | 'created_at'
  | 'howlongtobeat_main'
  | 'personal_rating'
  | 'release_date'
  | 'hours_played'
  | 'rating_average';

export type SortOrder = 'asc' | 'desc';

export interface SortOption {
  value: SortField;
  label: string;
}

export const sortOptions: SortOption[] = [
  { value: 'title', label: 'Title' },
  { value: 'created_at', label: 'Date Added' },
  { value: 'howlongtobeat_main', label: 'Time to Beat' },
  { value: 'personal_rating', label: 'My Rating' },
  { value: 'release_date', label: 'Release Date' },
  { value: 'hours_played', label: 'Hours Played' },
  { value: 'rating_average', label: 'IGDB Rating' },
];
